package server

import (
	"encoding/base64"
	"net/http"
	"strings"

	"qjrxy/internal/service"

	qrcode "github.com/skip2/go-qrcode"
)

// scanLoginClientIDs 是 authorize 允许的 client_id 候选。
//
// 16896453639520707 是 2026-10 实抓 HAR 里学校门户换票用的那个（长江师范实证）；
// 15996138616888888 是 PC 登录链路用的。不同学校对接的第三方应用不同，
// 学校认定哪一个是合法的只有试过才知道——所以按顺序降级，任一跳到学校域即算成功。
// 全部失败时手机页提示「该校暂不支持扫码登录」，而不是静默卡住。
var scanLoginClientIDs = []string{"16896453639520707", "15996138616888888"}

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
		"clientIds": scanLoginClientIDs,
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
	school, username, alias, err := s.svc.ScanLoginTicketTarget(r.Context(), ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"schoolName": school,
		"username":   username,
		"alias":      alias,
		"clientIds":  scanLoginClientIDs,
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
