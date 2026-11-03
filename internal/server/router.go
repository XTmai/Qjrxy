package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"qjrxy/internal/service"
)

// Server 持有 HTTP 服务依赖。
type Server struct {
	svc      *service.Service
	tokenTTL int
}

// New 创建 HTTP 服务。
func New(svc *service.Service) *Server {
	return &Server{svc: svc, tokenTTL: 30}
}

// Handler 返回配置好的路由。
func (s *Server) Handler(static http.Handler) http.Handler {
	mux := http.NewServeMux()

	// ---- 公开接口 ----
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/selftest", s.handleSelfTest)
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/me", s.handleMe)
	mux.HandleFunc("GET /api/schools", s.withUser(s.handleSchools))
	mux.HandleFunc("GET /api/system/info", s.withUser(s.handleSystemInfo))
	mux.HandleFunc("POST /api/auth/password", s.withUser(s.handleChangePassword))
	mux.HandleFunc("POST /api/device/submit", s.handleDeviceSubmit)

	// ---- 账号 ----
	mux.HandleFunc("GET /api/accounts", s.withUser(s.handleListAccounts))
	mux.HandleFunc("POST /api/accounts", s.withUser(s.handleCreateAccount))
	mux.HandleFunc("GET /api/accounts/{id}", s.withUser(s.handleGetAccount))
	mux.HandleFunc("PATCH /api/accounts/{id}", s.withUser(s.handleUpdateAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", s.withUser(s.handleDeleteAccount))
	mux.HandleFunc("POST /api/accounts/{id}/login", s.withUser(s.handleAccountLogin))
	mux.HandleFunc("POST /api/accounts/{id}/captcha", s.withUser(s.handleAccountCaptcha))
	mux.HandleFunc("POST /api/accounts/{id}/verify", s.withUser(s.handleVerifySession))
	mux.HandleFunc("POST /api/accounts/probe", s.withUser(s.handleProbeAccounts))
	mux.HandleFunc("POST /api/accounts/{id}/photo", s.withUser(s.handleUploadPhoto))
	mux.HandleFunc("GET /api/accounts/{id}/photo", s.withUser(s.handleGetPhoto))
	mux.HandleFunc("POST /api/accounts/{id}/device-ticket", s.withUser(s.handleDeviceTicket))
	mux.HandleFunc("GET /api/device/poll/{ticket}", s.withUser(s.handleDevicePoll))
	mux.HandleFunc("GET /api/accounts/{id}/sign-range", s.withUser(s.handleFetchRange))
	mux.HandleFunc("POST /api/accounts/{id}/sign-range", s.withUser(s.handleApplyRange))

	// ---- 任务 ----
	mux.HandleFunc("GET /api/tasks", s.withUser(s.handleListTasks))
	mux.HandleFunc("POST /api/tasks/sync", s.withUser(s.handleSyncTasks))
	mux.HandleFunc("POST /api/tasks/run", s.withUser(s.handleRunTask))

	// ---- 计划 ----
	mux.HandleFunc("GET /api/plans", s.withUser(s.handleListPlans))
	mux.HandleFunc("POST /api/plans", s.withUser(s.handleCreatePlan))
	mux.HandleFunc("PATCH /api/plans/{id}", s.withUser(s.handleUpdatePlan))
	mux.HandleFunc("DELETE /api/plans/{id}", s.withUser(s.handleDeletePlan))
	mux.HandleFunc("POST /api/plans/{id}/toggle", s.withUser(s.handleTogglePlan))
	mux.HandleFunc("POST /api/plans/{id}/reset", s.withUser(s.handleResetPlanRun))

	// ---- 记录 ----
	mux.HandleFunc("GET /api/logs", s.withUser(s.handleListLogs))
	mux.HandleFunc("GET /api/logs/trend", s.withUser(s.handleLogTrend))
	mux.HandleFunc("GET /api/logs/{id}", s.withUser(s.handleGetLog))

	// ---- 工作台 ----
	mux.HandleFunc("GET /api/dashboard", s.withUser(s.handleDashboard))

	// ---- 额度 ----
	mux.HandleFunc("POST /api/quota/redeem", s.withUser(s.handleRedeemQuota))

	if static != nil {
		mux.Handle("/", static)
	}
	return withLogging(mux)
}

// withUser 要求登录，并把用户放进上下文。
func (s *Server) withUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		user, err := s.svc.Authenticate(r.Context(), token)
		if err != nil {
			if token == "" {
				writeError(w, unauthorized("请先登录"))
				return
			}
			writeError(w, unauthorized("登录状态已失效，请重新登录"))
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser, user)
		next(w, r.WithContext(ctx))
	}
}

// pathID 读取路径中的 {id}。
func pathID(r *http.Request) (int64, error) {
	raw := r.PathValue("id")
	if strings.TrimSpace(raw) == "" {
		return 0, badRequest("缺少 id")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest("id 不合法")
	}
	return id, nil
}
