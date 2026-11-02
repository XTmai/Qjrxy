package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"qingjin/internal/store"
)

type ctxKey string

const ctxUser ctxKey = "user"

// APIError 是可以直接返回给客户端的错误。
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Message }

func badRequest(msg string) error { return &APIError{Status: 400, Code: "bad_request", Message: msg} }
func unauthorized(msg string) error { return &APIError{Status: 401, Code: "unauthorized", Message: msg} }
func notFound(msg string) error  { return &APIError{Status: 404, Code: "not_found", Message: msg} }

// respondJSON 写出统一结构：{ ok, data } / { ok:false, error }。
func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error *errObj `json:"error,omitempty"`
}

type errObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeData(w http.ResponseWriter, status int, data any) {
	respondJSON(w, status, envelope{OK: true, Data: data})
}

func writeError(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		respondJSON(w, apiErr.Status, envelope{OK: false, Error: &errObj{Code: apiErr.Code, Message: apiErr.Message}})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		respondJSON(w, 404, envelope{OK: false, Error: &errObj{Code: "not_found", Message: "记录不存在"}})
		return
	}
	respondJSON(w, 400, envelope{OK: false, Error: &errObj{Code: "failed", Message: err.Error()}})
}

// decodeJSON 读取并限制请求体大小。
func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return badRequest("请求体格式不正确：" + err.Error())
	}
	return nil
}

// userFrom 从上下文取当前用户。
func userFrom(ctx context.Context) (*store.User, bool) {
	u, ok := ctx.Value(ctxUser).(*store.User)
	return u, ok
}

// requireUser 取不到用户时返回未授权错误。
func requireUser(ctx context.Context) (*store.User, error) {
	u, ok := userFrom(ctx)
	if !ok || u == nil {
		return nil, unauthorized("请先登录")
	}
	return u, nil
}

// bearerToken 提取 Authorization 或 cookie 中的会话。
func bearerToken(r *http.Request) string {
	if raw := r.Header.Get("Authorization"); raw != "" {
		if len(raw) > 7 && strings.EqualFold(raw[:7], "bearer ") {
			return strings.TrimSpace(raw[7:])
		}
		return strings.TrimSpace(raw)
	}
	if ck, err := r.Cookie("qj_token"); err == nil {
		return ck.Value
	}
	return ""
}

// logging 记录请求耗时。
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
