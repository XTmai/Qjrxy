package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/events"
	"qjrxy/internal/secure"
	"qjrxy/internal/store"
)

// ---------- QR 扫码登录 ----------

// qrLoginSession 是一次扫码登录的会话状态。
type qrLoginSession struct {
	Ticket     string
	QRId       string
	Params     *cpdaily.QRLoginParams
	SchoolName string // 用户预选学校（可空）
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Status     string // waiting / scanned / success / expired
}

// qrSessions 是内存中的扫码会话（进程重启即清空，与 device-ticket 一致）。
var (
	qrMu       sync.Mutex
	qrSessions = map[string]*qrLoginSession{}
)

func qrCleanupExpired() {
	now := time.Now()
	for k, v := range qrSessions {
		if now.After(v.ExpiresAt) {
			delete(qrSessions, k)
		}
	}
}

// handleQRLoginStart 开始一次扫码登录。
//
// body: {"schoolName": "可选，预选学校"}
// 返回: {ticket, qrImage (base64 PNG), expiresAt}
func (s *Server) handleQRLoginStart(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		SchoolName string `json:"schoolName"`
	}
	_ = decodeJSON(r, &in)

	qrMu.Lock()
	qrCleanupExpired()
	qrMu.Unlock()

	// 获取 OAuth 参数：优先走 uniauth（实测对所有学校类型稳定）。
	// 学校名只用于拿到该校的 appId；扫码后的实际学校由 App 当前账号决定。
	appID := ""
	host := ""
	if in.SchoolName != "" {
		if tenant, terr := cpdaily.ResolveTenant(r.Context(), in.SchoolName); terr == nil {
			appID = tenant.AppID
			host = tenant.Host
		}
	}
	params, err := cpdaily.GetQROAuthParamsFromUniauth(r.Context(), appID)
	if err != nil && host != "" {
		// 兜底：从学校入口 302 里提取
		if p2, e2 := cpdaily.GetQROAuthParams(r.Context(), host); e2 == nil {
			params, err = p2, nil
		}
	}
	if err != nil {
		writeError(w, badRequest("获取扫码授权参数失败："+err.Error()))
		return
	}

	// 调用 jsLogin
	qrId, err := cpdaily.QRLoginStart(r.Context(), params)
	if err != nil {
		writeError(w, badRequest("生成二维码失败："+err.Error()))
		return
	}

	// 生成内部 ticket
	ticket := fmt.Sprintf("%x", time.Now().UnixNano())
	expiresAt := time.Now().Add(5 * time.Minute)

	qrMu.Lock()
	qrSessions[ticket] = &qrLoginSession{
		Ticket:     ticket,
		QRId:       qrId,
		Params:     params,
		SchoolName: in.SchoolName,
		UserID:     user.ID,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
		Status:     "waiting",
	}
	qrMu.Unlock()

	// 下载二维码图片并 base64
	qrURL := cpdaily.QRLoginImageURL(qrId)
	qrImage := ""
	if imgData, ierr := s.fetchQRImage(r.Context(), qrURL); ierr == nil {
		// campushoy 实际返回的是 JPEG，不能写死 png。
		mime := http.DetectContentType(imgData)
		if !strings.HasPrefix(mime, "image/") {
			mime = "image/png"
		}
		qrImage = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imgData)
	}

	writeData(w, http.StatusOK, map[string]any{
		"ticket":    ticket,
		"qrImage":   qrImage,
		"qrId":      qrId,
		"expiresAt": expiresAt.Format(time.RFC3339),
	})
}

// handleQRLoginPoll 轮询扫码状态。
//
// body: {"ticket": "..."}
// 返回: {status: "waiting"|"scanned"|"success"|"expired", account?: accountView}
func (s *Server) handleQRLoginPoll(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Ticket string `json:"ticket"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}

	qrMu.Lock()
	sess, ok := qrSessions[in.Ticket]
	qrMu.Unlock()
	if !ok || time.Now().After(sess.ExpiresAt) {
		writeData(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}

	// 轮询 campushoy validation
	res, err := cpdaily.QRLoginPoll(r.Context(), sess.QRId)
	if err != nil {
		// 服务端明确报错（errCode != 0）要终止轮询并告知用户；
		// 之前一律回 waiting，票据失效时用户会无限等下去。
		// 网络抖动仍然按 waiting 处理，下一轮再试。
		var apiErr *cpdaily.QRCodeAPIError
		if errors.As(err, &apiErr) {
			qrMu.Lock()
			delete(qrSessions, in.Ticket)
			qrMu.Unlock()
			writeError(w, badRequest("扫码登录失败："+apiErr.Error()))
			return
		}
		writeData(w, http.StatusOK, map[string]any{"status": "waiting"})
		return
	}

	switch res.Status {
	case 1:
		writeData(w, http.StatusOK, map[string]any{"status": "waiting"})
	case 2, 3:
		qrMu.Lock()
		sess.Status = "scanned"
		qrMu.Unlock()
		writeData(w, http.StatusOK, map[string]any{"status": "scanned"})
	case 5:
		qrMu.Lock()
		delete(qrSessions, in.Ticket)
		qrMu.Unlock()
		writeData(w, http.StatusOK, map[string]any{"status": "expired"})
	case 4:
		// 扫码确认成功！跟随跳转链获取 Cookie
		qrMu.Lock()
		sess.Status = "success"
		qrMu.Unlock()

		cookieResult, ferr := cpdaily.QRLoginFollow(r.Context(), res.RedirectURL)
		if ferr != nil {
			// 跳转链失败（典型：NOTCLOUD 学校链路绕进学校自建认证）。
			// OAuth 码已被这次失败消费掉，同一个码不可能重来——
			// 会话删掉并用 failed 状态如实告知，前端展示真实原因，
			// 不能让下一次轮询的 status=5 把它盖成「二维码已过期」。
			qrMu.Lock()
			delete(qrSessions, in.Ticket)
			qrMu.Unlock()
			writeData(w, http.StatusOK, map[string]any{
				"status":  "failed",
				"message": "获取登录态失败：" + ferr.Error(),
			})
			return
		}

		// 用 Cookie 创建或更新账号
		account, aerr := s.qrLoginSaveAccount(r.Context(), user.ID, cookieResult, sess.SchoolName)
		if aerr != nil {
			writeError(w, badRequest("保存账号失败："+aerr.Error()))
			return
		}

		qrMu.Lock()
		delete(qrSessions, in.Ticket)
		qrMu.Unlock()

		s.svc.Events().Publish(user.ID, events.TopicAccounts)
		writeData(w, http.StatusOK, map[string]any{
			"status":  "success",
			"account": toAccountView(account),
		})
	default:
		writeData(w, http.StatusOK, map[string]any{"status": "waiting"})
	}
}

// qrLoginSaveAccount 用扫码得到的 Cookie 创建或更新账号。
func (s *Server) qrLoginSaveAccount(ctx context.Context, userID int64, result *cpdaily.QRLoginCookies, schoolName string) (*store.Account, error) {
	host := result.TenantHost
	if host == "" {
		return nil, fmt.Errorf("无法识别学校域名")
	}

	// 提取学校域名的 Cookie（MOD_AUTH_CAS / CASTGC 等）
	schoolCookies := map[string]string{}
	for key, val := range result.Cookies {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		domain, name := parts[0], parts[1]
		if strings.Contains(domain, host) || strings.Contains(host, domain) {
			schoolCookies[name] = val
		}
	}

	// 解析租户（用 host 直接查，不需要精确名称）
	tenant, terr := cpdaily.ResolveTenant(ctx, host)
	if terr != nil {
		// 解析失败时用 host 构建一个最小化的租户信息
		tenant = &cpdaily.Tenant{Host: host, Name: host, JoinType: "CLOUD"}
	}

	// 用 TGC 调 validation 获取用户信息（学号/姓名）
	username, alias := s.qrExtractUserInfo(ctx, result, schoolCookies)

	// 构建 Cookie JSON
	cookieJSON, _ := json.Marshal(schoolCookies)

	// 查找已有账号或创建新账号
	existing, dbErr := s.svc.DB().AccountBySchoolUsername(ctx, userID, tenant.Name, username)
	if dbErr == nil {
		// 已有账号：更新 Cookie 和 TGC
		existing.Cookies = string(cookieJSON)
		existing.LoginStatus = "authorized"
		if err := s.svc.DB().UpdateAccountSession(ctx, existing.ID, string(cookieJSON), "authorized"); err != nil {
			return nil, err
		}
		if result.TGC != "" {
			tgcEnc, encErr := secure.Encrypt(s.svc.Config().SecretKey, result.TGC)
			if encErr == nil {
				if err := s.svc.DB().UpdateAccountTGC(ctx, existing.ID, tgcEnc); err != nil {
					return nil, err
				}
			}
		}
		return existing, nil
	}

	// 新账号
	account := &store.Account{
		UserID:      userID,
		Alias:       alias,
		SchoolName:  tenant.Name,
		Username:    username,
		TenantID:    tenant.ID,
		Host:        tenant.Host,
		IDSURL:      tenant.IDSURL,
		JoinType:    tenant.JoinType,
		Cookies:     string(cookieJSON),
		LoginStatus: "authorized",
	}
	if result.TGC != "" {
		tgcEnc, encErr := secure.Encrypt(s.svc.Config().SecretKey, result.TGC)
		if encErr == nil {
			account.TGCEnc = tgcEnc
		}
	}
	created, err := s.svc.DB().CreateAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// qrExtractUserInfo 尝试从扫码 Cookie 中提取学号和别名。
// 简化实现：用 host + 时间戳生成临时标识，用户可在编辑页修改。
func (s *Server) qrExtractUserInfo(ctx context.Context, result *cpdaily.QRLoginCookies, schoolCookies map[string]string) (username, alias string) {
	// TODO: 后续可加调 validation 端点获取真实学号/姓名
	return fmt.Sprintf("qr_%d", time.Now().UnixMilli()%1000000), "扫码用户"
}

// fetchQRImage 下载二维码图片。
func (s *Server) fetchQRImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("QR 图片下载失败: %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 最多 1MB
}
