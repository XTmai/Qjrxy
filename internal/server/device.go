package server

import (
	"encoding/base64"
	"net/http"
	"strings"

	"qjrxy/internal/service"

	qrcode "github.com/skip2/go-qrcode"
)

// handleDeviceTicket 为账号生成"扫码获取设备 ID"的票据与二维码。
func (s *Server) handleDeviceTicket(w http.ResponseWriter, r *http.Request) {
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
	ticket, expires, err := s.svc.CreateDeviceTicket(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	// 设备页需要用绝对地址，拼接当前请求的 host。
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	deviceURL := scheme + "://" + host + "/device.html?t=" + ticket

	var png []byte
	if qr, err := qrcode.New(deviceURL, qrcode.Medium); err == nil {
		qr.DisableBorder = false
		png, _ = qr.PNG(360)
	}
	writeData(w, http.StatusOK, map[string]any{
		"ticket":    ticket,
		"url":       deviceURL,
		"expiresAt": expires,
		"qrcode":    "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	})
}

// handleDeviceSubmit 由扫码页调用：提交设备信息（票据授权，无需登录）。
func (s *Server) handleDeviceSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ticket        string `json:"ticket"`
		DeviceID      string `json:"deviceId"`
		UUID          string `json:"uuid"`
		DeviceModel   string `json:"deviceModel"`
		SystemVersion string `json:"systemVersion"`
		BuildID       string `json:"buildId"`
		SystemName    string `json:"systemName"`
		AppVersion    string `json:"appVersion"`
		WisDeviceID   string `json:"wisDeviceId"`
		UserAgent     string `json:"userAgent"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	err := s.svc.SubmitDeviceInfo(req.Ticket, service.DeviceInfo{
		DeviceID:      req.DeviceID,
		UUID:          req.UUID,
		DeviceModel:   req.DeviceModel,
		SystemVersion: req.SystemVersion,
		BuildID:       req.BuildID,
		SystemName:    req.SystemName,
		AppVersion:    req.AppVersion,
		WisDeviceID:   req.WisDeviceID,
		UserAgent:     req.UserAgent,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"message": "设备信息已提交，请回到电脑端确认",
	})
}

// handleDevicePoll 电脑端轮询扫码结果。
func (s *Server) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	ticket := r.PathValue("ticket")
	if ticket == "" {
		writeError(w, badRequest("缺少票据"))
		return
	}
	pending, info, err := s.svc.PollDeviceTicket(ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	if pending || info == nil {
		writeData(w, http.StatusOK, map[string]any{"pending": true})
		return
	}
	// 拿到结果就落库，避免前端多一次请求
	account, err := s.svc.CollectDeviceInfo(r.Context(), user.ID, ticket)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"pending":       false,
		"collected":     true,
		"deviceId":      account.DeviceID,
		"deviceModel":   account.DeviceModel,
		"systemVersion": account.SystemVersion,
		"buildId":       account.BuildID,
		"platform":      account.Platform,
		"wisDeviceId":   account.WisDeviceID,
		"uuid":          info.UUID,
		"account":       account,
	})
}

// handleChangePassword 修改平台密码。
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := s.svc.ChangePassword(r.Context(), user.ID, req.OldPassword, req.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"message": "密码已更新"})
}

// handleFetchRange 自动获取签到范围（只读，不写库）。
func (s *Server) handleFetchRange(w http.ResponseWriter, r *http.Request) {
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
	rng, err := s.svc.FetchSignRange(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"range": rng})
}

// handleApplyRange 自动获取签到范围并写入账号。
func (s *Server) handleApplyRange(w http.ResponseWriter, r *http.Request) {
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
	account, rng, err := s.svc.ApplySignRange(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"account": toAccountView(account),
		"range":   rng,
		"message": "已按学校签到点自动设置位置",
	})
}
