// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"qjrxy/internal/events"
	"qjrxy/internal/service"
	"qjrxy/internal/store"
)

// accountView 是返回给前端的账号结构（不含密码/cookie）。
type accountView struct {
	store.Account
	HasCookie bool   `json:"hasCookie"`
	PhotoURL  string `json:"photoUrl"`
	// PhotoPoolCount 是该账号照片池的数量；photoUrl 在单张为空时会
	// 自动指向池内第一张，账号卡片/首页缩略图无需区分两种来源。
	PhotoPoolCount int `json:"photoPoolCount"`
}

func toAccountView(a *store.Account) accountView {
	view := accountView{Account: *a, HasCookie: a.HasCookie()}
	if a.PhotoPath != "" {
		view.PhotoURL = "/api/accounts/" + itoa(a.ID) + "/photo"
	}
	return view
}

// applyPoolStats 把照片池数量与封面写进账号视图。
func applyPoolStats(view *accountView, accountID int64, stats store.PhotoPoolStats) {
	view.PhotoPoolCount = stats.Count
	if view.PhotoURL == "" && stats.FirstID > 0 {
		view.PhotoURL = accountPhotoURL(accountID, stats.FirstID)
	}
}

// accountViews 批量填充照片池信息（一次 GROUP BY，避免逐账号查询）。
func (s *Server) accountViews(ctx context.Context, userID int64, accounts []store.Account) []accountView {
	stats, _ := s.svc.DB().PhotoPoolStatsByUser(ctx, userID)
	views := make([]accountView, 0, len(accounts))
	for i := range accounts {
		view := toAccountView(&accounts[i])
		applyPoolStats(&view, accounts[i].ID, stats[accounts[i].ID])
		views = append(views, view)
	}
	return views
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
	writeData(w, http.StatusOK, map[string]any{"accounts": s.accountViews(r.Context(), user.ID, accounts)})
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
	account, existed, err := s.svc.CreateAccount(r.Context(), user.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	// existed=true 表示该学校下学号本来就已绑定，本次只是更新了密码/设备信息。
	// 返回 200 而不是 201，前端据此提示「账号已存在」并继续登录。
	status := http.StatusCreated
	if existed {
		status = http.StatusOK
	}
	writeData(w, status, map[string]any{"account": toAccountView(account), "existed": existed})
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
	view := toAccountView(account)
	if stats, err := s.svc.DB().PhotoPoolStats(r.Context(), user.ID, account.ID); err == nil {
		applyPoolStats(&view, account.ID, stats)
	}
	detail := map[string]any{"account": view}

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
	// 账号行删除后池记录会级联清空；文件不受数据库约束，单独清理整目录。
	s.svc.RemoveAccountPhotoDir(user.ID, id)
	s.svc.Events().Publish(user.ID, events.TopicAccounts)
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

// handleSMSSend 为「学校自建统一身份认证」的账号发送短信验证码。
func (s *Server) handleSMSSend(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Phone string `json:"phone"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	ticket, err := s.svc.BeginSMSLogin(r.Context(), user.ID, id, req.Phone)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"done":      false,
		"ticket":    ticket.ID,
		"phone":     ticket.Phone,
		"expiresAt": ticket.ExpiresAt,
		"message":   "验证码已发送，请注意查收",
	})
}

// handleSMSLogin 用短信验证码完成登录。
func (s *Server) handleSMSLogin(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Ticket) == "" || strings.TrimSpace(req.Code) == "" {
		writeError(w, badRequest("验证码与会话标识不能为空"))
		return
	}
	account, err := s.svc.SubmitSMSLogin(r.Context(), user.ID, id, req.Ticket, req.Code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"done": true, "message": "登录成功", "account": toAccountView(account)})
}

// handleProbeAccounts 手动触发登录态探测（单个或全部账号）。
//
// 探测会顺带同步任务：会话有效则更新任务缓存，失效则自动用保存的密码重登。
func (s *Server) handleProbeAccounts(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		AccountID int64 `json:"accountId"` // 0 表示全部账号
	}
	_ = decodeJSON(r, &req)

	ctx := r.Context()
	var results []service.ProbeResult
	if req.AccountID > 0 {
		account, err := s.svc.DB().AccountByID(ctx, user.ID, req.AccountID)
		if err != nil {
			writeError(w, err)
			return
		}
		results = append(results, s.svc.ProbeAccount(ctx, account))
	} else {
		accounts, err := s.svc.DB().ListAccounts(ctx, user.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		for i := range accounts {
			results = append(results, s.svc.ProbeAccount(ctx, &accounts[i]))
		}
	}
	writeData(w, http.StatusOK, map[string]any{"results": results})
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
	data, err := io.ReadAll(io.LimitReader(file, 6<<20))
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
	// 只更新照片字段：这里拿到的 account 是 multipart 解析前的快照，
	// 整行 UPDATE 会把并发写入的 cookies / login_status / device_id / 坐标回滚成旧值。
	if err := s.svc.DB().UpdateAccountPhotoPath(r.Context(), user.ID, account.ID, rel); err != nil {
		writeError(w, err)
		return
	}
	account.PhotoPath = rel
	s.svc.Events().Publish(user.ID, events.TopicAccounts)
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
	s.servePhotoFile(w, r, path)
}

// servePhotoFile 输出照片，支持 ?w= 缩略图与长缓存（单张与照片池共用）。
func (s *Server) servePhotoFile(w http.ResponseWriter, r *http.Request, path string) {
	// ?w=96/320 返回缩略图：手机原图通常 3-5MB / 3000×4000px，列表页 12 张
	// 原图直出会让浏览器解码上亿像素，页面明显卡顿。缩略图按 JPEG 重新
	// 编码（EXIF 方向已烘焙进像素），URL 带 t=updatedAt 版本号，可长缓存。
	if wStr := r.URL.Query().Get("w"); wStr != "" {
		tw, err := strconv.Atoi(wStr)
		if err != nil || tw < 16 || tw > 1280 {
			writeError(w, badRequest("w 参数需在 16-1280 之间"))
			return
		}
		if thumb, terr := photoThumbnail(path, tw); terr == nil {
			w.Header().Set("Content-Type", "image/jpeg")
			// 前端用 t=updatedAt 做缓存破坏，URL 内容不可变，可以长缓存
			w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
			_, _ = w.Write(thumb)
			return
		}
		// 生成失败（如 webp）：退回原图
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}

// helpers

func itoa(v int64) string {
	buf, _ := json.Marshal(v)
	return strings.Trim(string(buf), `"`)
}

var _ = errors.Is
var _ = filepath.Base
