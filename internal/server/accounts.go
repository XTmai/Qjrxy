package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"qingjin/internal/service"
	"qingjin/internal/store"
)

// accountView 是返回给前端的账号结构（不含密码/cookie）。
type accountView struct {
	store.Account
	HasCookie bool   `json:"hasCookie"`
	PhotoURL  string `json:"photoUrl"`
}

func toAccountView(a *store.Account) accountView {
	view := accountView{Account: *a, HasCookie: a.HasCookie()}
	if a.PhotoPath != "" {
		view.PhotoURL = "/api/accounts/" + itoa(a.ID) + "/photo"
	}
	return view
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	accounts, err := s.svc.DB().ListAccounts(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]accountView, 0, len(accounts))
	for i := range accounts {
		views = append(views, toAccountView(&accounts[i]))
	}
	writeData(w, http.StatusOK, map[string]any{"accounts": views})
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var in service.AccountInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	account, err := s.svc.CreateAccount(r.Context(), user.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"account": toAccountView(account)})
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
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
	account, err := s.svc.DB().AccountByID(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	detail := map[string]any{"account": toAccountView(account)}

	// 附带该账号的待办任务（缓存）
	if tasks, err := s.svc.DB().TasksByAccount(r.Context(), account.ID); err == nil {
		detail["tasks"] = tasks
	}
	writeData(w, http.StatusOK, detail)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
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
	var in service.AccountInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	account, err := s.svc.UpdateAccount(r.Context(), user.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"account": toAccountView(account)})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
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
	if err := s.svc.DB().DeleteAccount(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"message": "账号已删除"})
}

func (s *Server) handleAccountLogin(w http.ResponseWriter, r *http.Request) {
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
	ticket, err := s.svc.BeginLogin(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if ticket.ID == "" {
		writeData(w, http.StatusOK, map[string]any{"done": true, "message": "登录成功"})
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"done":        false,
		"ticket":      ticket.ID,
		"needCaptcha": ticket.NeedCaptcha,
		"captcha":     service.CaptchaDataURL(ticket.Captcha),
		"expiresAt":   ticket.ExpiresAt,
		"message":     "请输入验证码完成登录",
	})
}

type captchaRequest struct {
	Ticket string `json:"ticket"`
	Code   string `json:"code"`
}

func (s *Server) handleAccountCaptcha(w http.ResponseWriter, r *http.Request) {
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
	var req captchaRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Ticket) == "" || strings.TrimSpace(req.Code) == "" {
		writeError(w, badRequest("验证码与会话标识不能为空"))
		return
	}
	if err := s.svc.SubmitCaptcha(r.Context(), user.ID, id, req.Ticket, strings.TrimSpace(req.Code)); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"done": true, "message": "登录成功"})
}

type cookieRequest struct {
	Cookies string `json:"cookies"`
}

func (s *Server) handleImportCookies(w http.ResponseWriter, r *http.Request) {
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
	var req cookieRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	status, err := s.svc.ImportCookies(r.Context(), user.ID, id, req.Cookies)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"loginStatus": status, "message": "Cookie 已保存"})
}

func (s *Server) handleVerifySession(w http.ResponseWriter, r *http.Request) {
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
	status, err := s.svc.VerifySession(r.Context(), user.ID, id)
	if err != nil {
		writeData(w, http.StatusOK, map[string]any{"loginStatus": status, "message": err.Error()})
		return
	}
	writeData(w, http.StatusOK, map[string]any{"loginStatus": status, "message": "登录状态正常"})
}

// handleUploadPhoto 接收 multipart 照片。
func (s *Server) handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
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
	account, err := s.svc.DB().AccountByID(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, badRequest("照片上传失败："+err.Error()))
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, badRequest("未找到照片文件"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 9<<20))
	if err != nil {
		writeError(w, badRequest("读取照片失败"))
		return
	}
	rel, err := s.svc.SavePhoto(user.ID, account.ID, header.Filename, data)
	if err != nil {
		writeError(w, err)
		return
	}
	// 删除旧照片
	if account.PhotoPath != "" && account.PhotoPath != rel {
		_ = os.Remove(s.svc.PhotoAbsPath(account.PhotoPath))
	}
	account.PhotoPath = rel
	if err := s.svc.DB().UpdateAccount(r.Context(), account); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"account": toAccountView(account)})
}

func (s *Server) handleGetPhoto(w http.ResponseWriter, r *http.Request) {
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
	account, err := s.svc.DB().AccountByID(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	path := s.svc.PhotoAbsPath(account.PhotoPath)
	if path == "" {
		writeError(w, notFound("尚未绑定照片"))
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, notFound("照片文件不存在"))
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeFile(w, r, path)
}

// handleRedeemQuota 兑换额度卡密。
func (s *Server) handleRedeemQuota(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	amount := redeemAmount(req.Code)
	if amount <= 0 {
		writeError(w, badRequest("卡密无效或已被使用"))
		return
	}
	if err := s.svc.DB().GrantQuota(r.Context(), user.ID, amount); err != nil {
		writeError(w, err)
		return
	}
	updated, err := s.svc.DB().UserByID(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"user": updated, "granted": amount, "message": "兑换成功"})
}

// redeemAmount 校验卡密格式。演示实现：QJ-XXXX-XXXX 形式的卡密固定兑换 30 次。
func redeemAmount(code string) int {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !strings.HasPrefix(code, "QJ-") {
		return 0
	}
	parts := strings.Split(strings.TrimPrefix(code, "QJ-"), "-")
	if len(parts) != 2 || len(parts[0]) != 4 || len(parts[1]) != 4 {
		return 0
	}
	for _, p := range parts {
		for _, ch := range p {
			if !strings.ContainsRune("0123456789ABCDEF", ch) {
				return 0
			}
		}
	}
	return 30
}

// helpers

func itoa(v int64) string {
	buf, _ := json.Marshal(v)
	return strings.Trim(string(buf), `"`)
}

var _ = errors.Is
var _ = filepath.Base
