package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/secure"
	"qjrxy/internal/service"
	"qjrxy/internal/store"
)

// Server 持有 HTTP 服务依赖。
type Server struct {
	svc      *service.Service
	tokenTTL int
	// 登录/注册限流：登录 5 次/分钟、注册 3 次/小时（按 IP + 账号名）。
	loginLimiter    *rateLimiter
	registerLimiter *rateLimiter

	// 单用户模式缓存的本地用户（懒加载，见 ensureSingleUser）。
	singleOnce sync.Once
	singleUser *store.User
	singleErr  error

	// done 在进程退出时关闭，用来结束长连接的 SSE 推送。
	done     chan struct{}
	doneOnce sync.Once
}

// New 创建 HTTP 服务。
func New(svc *service.Service) *Server {
	return &Server{
		svc:             svc,
		tokenTTL:        30,
		loginLimiter:    newRateLimiter(5, time.Minute),
		registerLimiter: newRateLimiter(3, time.Hour),
		done:            make(chan struct{}),
	}
}

// Close 结束所有 SSE 长连接，让优雅退出不必等满超时。
func (s *Server) Close() {
	s.doneOnce.Do(func() { close(s.done) })
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
	// 实时变更推送（SSE）：页面靠它做到「数据变了不用刷新」。
	mux.HandleFunc("GET /api/events", s.withUser(s.handleEvents))
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
	mux.HandleFunc("POST /api/accounts/{id}/sms/send", s.withUser(s.handleSMSSend))
	mux.HandleFunc("POST /api/accounts/{id}/sms/login", s.withUser(s.handleSMSLogin))
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
	mux.HandleFunc("GET /api/logs/audit", s.withUser(s.handleAuditLogs))
	mux.HandleFunc("GET /api/logs/{id}", s.withUser(s.handleGetLog))

	// ---- 工作台 ----
	mux.HandleFunc("GET /api/stats/7d", s.withUser(s.handleStats7d))
	mux.HandleFunc("GET /api/dashboard", s.withUser(s.handleDashboard))

	// ---- 额度 ----
	mux.HandleFunc("POST /api/quota/redeem", s.withUser(s.handleRedeemQuota))

	// ---- 管理员（role=admin 或单用户模式；页面与其余用户隔离） ----
	mux.HandleFunc("GET /api/admin/config", s.withUser(s.requireAdmin(s.handleAdminGetConfig)))
	mux.HandleFunc("POST /api/admin/config", s.withUser(s.requireAdmin(s.handleAdminSaveConfig)))

	if static != nil {
		mux.Handle("/", static)
	}
	return withLogging(mux)
}

// withUser 要求登录，并把用户放进上下文。
// 单用户模式下（cfg.SingleUser）所有请求自动落到唯一的本地用户，
// 无需登录——本地双击 exe 即用的场景。
func (s *Server) withUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		user, err := s.svc.Authenticate(r.Context(), token)
		if err != nil && s.svc.Config().SingleUser {
			if user, err = s.ensureSingleUser(r.Context()); err != nil {
				writeError(w, &APIError{Status: http.StatusInternalServerError, Code: "failed", Message: err.Error()})
				return
			}
		}
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

// ensureSingleUser 返回单用户模式下唯一的本地用户：
// 优先复用库里已有用户（老数据无缝迁移），否则自动创建 admin。
// 结果缓存在 Server 实例上，避免每个请求都查库。
func (s *Server) ensureSingleUser(ctx context.Context) (*store.User, error) {
	s.singleOnce.Do(func() {
		user, err := s.svc.DB().FirstUser(ctx)
		if errors.Is(err, store.ErrNotFound) {
			// 随机密码：单用户模式不走登录，密码仅用于占位，无法被撞库
			user, err = s.svc.Register(ctx, "admin", secure.RandomID(32), "本地用户")
			if err != nil {
				s.singleErr = fmt.Errorf("创建本地用户失败：%w", err)
				return
			}
			log.Printf("单用户模式：已自动创建本地用户 %s（无需登录，打开即用）", user.Username)
		} else if err != nil {
			s.singleErr = err
			return
		}
		s.singleUser = user
	})
	if s.singleErr != nil {
		return nil, s.singleErr
	}
	return s.singleUser, nil
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
