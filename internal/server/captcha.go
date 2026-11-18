// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"errors"
	"net/http"

	"qjrxy/internal/captcha"
)

// handleCaptchaPending 列出该用户当前仍在等待人工处理的验证码。
//
// 浏览器弹窗与刷新恢复都读这里。返回的只有展示所需信息
// （目标词、九宫格图、截止时间），提交用的 code 与会话码不出后端。
func (s *Server) handleCaptchaPending(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items := s.svc.CaptchaPending(user.ID)
	if items == nil {
		items = []*captcha.Pending{}
	}
	writeData(w, http.StatusOK, items)
}

// handleCaptchaSubmit 回传人工判定结果：选中格子 / 换一张 / 取消。
func (s *Server) handleCaptchaSubmit(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	token := r.PathValue("token")
	var req struct {
		Cells  []int  `json:"cells"`
		Action string `json:"action"` // 空 = 提交选中；skip / cancel
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}

	switch req.Action {
	case "cancel":
		err = s.svc.CancelCaptcha(user.ID, token)
	case "skip":
		err = s.svc.SkipCaptcha(user.ID, token)
	case "":
		err = s.svc.SubmitCaptchaCells(user.ID, token, req.Cells)
	default:
		writeError(w, badRequest("不支持的操作："+req.Action))
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, captcha.ErrNotFound), errors.Is(err, captcha.ErrExpired):
			// 410 而不是 404：前端据此知道「这张已经没用了」，
			// 应该收起弹窗等下一张，而不是当成网络错误重试。
			writeError(w, &APIError{Status: http.StatusGone, Code: "captcha_gone", Message: err.Error()})
		case errors.Is(err, captcha.ErrNoSelection):
			writeError(w, badRequest(err.Error()))
		default:
			writeError(w, err)
		}
		return
	}
	writeData(w, http.StatusOK, map[string]any{"submitted": true})
}
