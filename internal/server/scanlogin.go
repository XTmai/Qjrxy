package server

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"

	"qjrxy/internal/service"

	qrcode "github.com/skip2/go-qrcode"
)

// handleScanLoginTicket 为账号生成「扫码获取登录态」的票据与二维码。
func (s *Server) handleScanLoginTicket(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ticket, expires, err := s.svc.CreateScanLoginTicket(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	account, err := s.svc.DB().AccountByID(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}

	scanURL := absoluteURL(r) + "/scan-login.html?t=" + ticket

	var png []byte
	if qr, err := qrcode.New(scanURL, qrcode.Medium); err == nil {
		qr.DisableBorder = false
		png, _ = qr.PNG(360)
	}
	writeData(w, http.StatusOK, map[string]any{
		"ticket":    ticket,
		"url":       scanURL,
		"expiresAt": expires,
		"qrcode":    "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"target": map[string]string{
			"schoolName": account.SchoolName,
			"username":   account.Username,
			"alias":      account.Alias,
			"host":       account.Host,
		},
	})
}

// handleScanLoginInfo 供手机扫码页读取「即将绑定到哪条记录」。
//
// 票据即授权，因此免登录；但只回三个展示字段，绝不带出 cookie、密码或 host。
func (s *Server) handleScanLoginInfo(w http.ResponseWriter, r *http.Request) {
	ticket := strings.TrimSpace(r.URL.Query().Get("t"))
	if ticket == "" {
		writeError(w, badRequest("缺少票据"))
		return
	}
	school, username, alias, host, storedAppUserID, err := s.svc.ScanLoginTicketTarget(r.Context(), ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	// client_id 每所学校一份，必须向该校 cpdailyLogin 端点要，不能写死。
	// 拿不到就明确告知「该校不支持扫码登录」，让手机页立刻停下，
	// 而不是拿别校的 client_id 去撞 500。
	oauth, err := s.svc.ResolveScanLoginOAuth(r.Context(), host)
	if err != nil {
		writeError(w, &APIError{Status: http.StatusBadGateway, Code: "oauth_unavailable",
			Message: "该校暂不支持扫码登录：" + err.Error()})
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"schoolName":     school,
		"username":       username,
		"alias":          alias,
		"host":           host,
		"clientId":       oauth.ClientID,
		"redirectUri":    oauth.RedirectURI,
		"state":          oauth.State,
		"storedAppUserId": storedAppUserID,
	})
}

// handleScanLoginSubmit 由扫码页调用：提交换到的学校票据（票据授权，免登录）。
func (s *Server) handleScanLoginSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ticket    string `json:"ticket"`
		ModAuth   string `json:"modAuthCas"`
		AppUserID string `json:"appUserId"`
		Host      string `json:"host"`
		School    string `json:"school"`
		Device    struct {
			DeviceID      string `json:"deviceId"`
			DeviceModel   string `json:"deviceModel"`
			SystemVersion string `json:"systemVersion"`
			BuildID       string `json:"buildId"`
			SystemName    string `json:"systemName"`
			AppVersion    string `json:"appVersion"`
			WisDeviceID   string `json:"wisDeviceId"`
			UserAgent     string `json:"userAgent"`
		} `json:"device"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	err := s.svc.SubmitScanLogin(req.Ticket, service.ScanLoginResult{
		Ticket:    req.ModAuth,
		AppUserID: req.AppUserID,
		Host:      req.Host,
		School:    req.School,
		Device: service.DeviceInfo{
			DeviceID:      req.Device.DeviceID,
			DeviceModel:   req.Device.DeviceModel,
			SystemVersion: req.Device.SystemVersion,
			BuildID:       req.Device.BuildID,
			SystemName:    req.Device.SystemName,
			AppVersion:    req.Device.AppVersion,
			WisDeviceID:   req.Device.WisDeviceID,
			UserAgent:     req.Device.UserAgent,
		},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"message": "登录态已提交，请回到电脑端确认",
	})
}

// handleScanLoginPoll 电脑端轮询扫码登录结果。
func (s *Server) handleScanLoginPoll(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("t"))
	if ticket == "" {
		writeError(w, badRequest("缺少票据"))
		return
	}
	// 票据必须属于这个账号：用户可能同时开了多个扫码弹窗，串号会把
	// A 账号的登录态写进 B 账号。
	if err := s.svc.ScanLoginTicketBelongsTo(r.Context(), user.ID, id, ticket); err != nil {
		writeError(w, err)
		return
	}
	pending, result, err := s.svc.PollScanLogin(ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	if pending || result == nil {
		writeData(w, http.StatusOK, map[string]any{"pending": true})
		return
	}
	// 拿到结果就落库，避免前端多一次请求。
	account, err := s.svc.CollectScanLogin(r.Context(), user.ID, ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"pending":   false,
		"collected": true,
		"account":   toAccountView(account),
	})
}

// handleScanLoginDiag 接收扫码页回传的诊断信息。
//
// 手机页的原生桥请求（MAMPHTTP）不经过我们的服务端，diag 抓不到；
// 换票失败时服务端只看到「没有票据」，完全不知道 App 里发生了什么。
// 这个端点让页面把每一步的落点地址、响应头、错误码回传，写进服务端日志。
//
// 只写诊断日志，不落库、不参与业务判定；票据类字段一律脱敏。
func (s *Server) handleScanLoginDiag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stage string `json:"stage"`
		Text  string `json:"text"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	// 防日志被灌爆
	if len(req.Text) > 4000 {
		req.Text = req.Text[:4000]
	}
	log.Printf("[scan-login diag] stage=%s %s", req.Stage, sanitizeScanDiag(req.Text))
	writeData(w, http.StatusOK, map[string]bool{"received": true})
}

// sanitizeScanDiag 把可能出现的票据、cookie_token 值打码。
//
// MOD_AUTH_CAS / cookieToken 都是等价于登录态的东西，进日志就等于泄露：
// 日志会被下载、贴到工单里、长期留存。只保留长度与前后几位用于比对。
func sanitizeScanDiag(s string) string {
	repl := func(s, key string) string {
		idx := 0
		for {
			i := strings.Index(s[idx:], key)
			if i < 0 {
				return s
			}
			start := idx + i + len(key)
			end := start
			for end < len(s) && s[end] != '"' && s[end] != ';' && s[end] != '&' &&
				s[end] != ' ' && s[end] != '}' {
				end++
			}
			if end-start <= 8 {
				idx = end
				continue
			}
			v := s[start:end]
			s = s[:start] + maskSecret(v) + s[end:]
			idx = start + len(maskSecret(v))
		}
	}
	for _, k := range []string{"MOD_AUTH_CAS=", "cookieToken\":\"", "cookieToken="} {
		s = repl(s, k)
	}
	return s
}

// maskSecret 只露头尾，中间打码。
func maskSecret(v string) string {
	if len(v) <= 12 {
		return "***"
	}
	return v[:6] + "…" + v[len(v)-4:] + fmt.Sprintf("(len=%d)", len(v))
}
