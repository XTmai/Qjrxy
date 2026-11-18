// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"qjrxy/internal/buildinfo"
)

// handleEvents 用 Server-Sent Events 把「哪类数据变了」推给浏览器。
//
// 只发主题不发业务数据：前端收到后让对应查询失效并重新拉取，
// 鉴权与字段过滤仍然只走各自的 HTTP 接口。这样推送通道即使被
// 代理缓存/截断，也不会泄露任何账号信息，重连后前端全量对齐即可。
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, &APIError{Status: http.StatusInternalServerError, Code: "stream_unsupported", Message: "当前环境不支持流式响应"})
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	// no-transform 防止中间层压缩/改写；EventSource 语义要求不缓存。
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("Connection", "keep-alive")
	// 宝塔/nginx 默认缓冲上游响应，SSE 会被攒着不下发。
	// 这个响应头让 nginx 对本次连接关闭缓冲。
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, unsubscribe := s.svc.Events().Subscribe(user.ID)
	defer unsubscribe()

	// 先告诉前端「连接已就绪」，顺便带上版本，重连后可发现服务已升级。
	if err := writeSSE(w, "ready", map[string]any{
		"appVersion": buildinfo.Version,
		"serverTime": time.Now().Format(time.RFC3339),
		"retryMs":    3000,
	}); err != nil {
		return
	}
	flusher.Flush()

	// 心跳同时承担断线探测：写失败说明连接已经没了，及时退出释放订阅。
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			// 进程正在退出：主动收尾，避免优雅关闭被长连接拖住。
			return
		case ev := <-ch:
			if err := writeSSE(w, "invalidate", ev); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSE 写出一条完整事件（event + data + 空行分隔）。
func writeSSE(w io.Writer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}
