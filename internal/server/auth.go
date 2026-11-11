package server

import (
	"context"
	"net/http"
	"time"

	"qjrxy/internal/buildinfo"
	"qjrxy/internal/cpdaily"
	"qjrxy/internal/diag"
	"qjrxy/internal/secure"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
		"name":   "Qjrxy · 今日校园签到与查寝",
	})
}

// handleSelfTest 校验加密链路与数据库连通性，便于部署后自检。
func (s *Server) handleSelfTest(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{
		"crypto": "ok", "database": "ok",
		"appVersion": buildinfo.Version, "clientVersion": cpdaily.AppVersion,
	}
	if err := cpdaily.CryptoSelfTest(); err != nil {
		result["crypto"] = err.Error()
	}
	if _, err := s.svc.DB().CountUsers(r.Context()); err != nil {
		result["database"] = err.Error()
	}
	status := http.StatusOK
	if result["crypto"] != "ok" || result["database"] != "ok" {
		status = http.StatusInternalServerError
	}
	writeData(w, status, result)
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// 单用户模式没有账号体系：前端根本不会进登录页，注册口直接关闭，
	// 避免有人对无鉴权的本地实例发起注册。
	if s.svc.Config().SingleUser {
		writeError(w, &APIError{Status: http.StatusForbidden, Code: "single_user", Message: "单用户模式无需注册，打开即用；服务器多人使用请设置 QINGJIN_MULTI_USER=true"})
		return
	}
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	// 注册口默认只在「库里还没有任何用户」时开放（首次初始化），
	// 之后自动关闭，避免公网实例被陌生人注册占用。
	// 需要多人自助注册的场景显式设置 QINGJIN_REGISTER_OPEN=true。
	if allowed, reason := s.registerAllowed(r.Context()); !allowed {
		writeError(w, &APIError{Status: http.StatusForbidden, Code: "register_closed", Message: reason})
		return
	}
	if ok, wait := s.registerLimiter.allow(authLimitKey(r, req.Username), time.Now()); !ok {
		writeError(w, tooMany(wait))
		return
	}
	user, err := s.svc.Register(r.Context(), req.Username, req.Password, req.Nickname)
	if err != nil {
		writeError(w, err)
		return
	}
	token, err := s.issueSession(r, user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"user": user, "token": token})
}

// registerAllowed 判断当前是否允许注册。
func (s *Server) registerAllowed(ctx context.Context) (bool, string) {
	if s.svc.Config().RegisterOpen {
		return true, ""
	}
	count, err := s.svc.DB().CountUsers(ctx)
	if err != nil {
		return false, "暂时无法确认注册状态，请稍后重试"
	}
	if count == 0 {
		return true, "" // 首次初始化：允许创建第一个账号
	}
	return false, "注册已关闭：本实例仅限受邀使用。如需开放自助注册，请设置 QINGJIN_REGISTER_OPEN=true"
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	// 登录限流：挡住针对单账号的撞库，避免无限次尝试。
	limitKey := authLimitKey(r, req.Username)
	if ok, wait := s.loginLimiter.allow(limitKey, time.Now()); !ok {
		writeError(w, tooMany(wait))
		return
	}
	user, token, err := s.svc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	s.loginLimiter.reset(limitKey)
	s.setTokenCookie(w, token)
	writeData(w, http.StatusOK, map[string]any{"user": user, "token": token})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token != "" {
		_ = s.svc.Logout(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "qj_token", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeData(w, http.StatusOK, map[string]any{"message": "已退出登录"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.svc.Authenticate(r.Context(), bearerToken(r))
	if err != nil && s.svc.Config().SingleUser {
		// 单用户模式：无 token 也返回本地用户，前端跳过登录页
		if user, err = s.ensureSingleUser(r.Context()); err != nil {
			writeError(w, &APIError{Status: http.StatusInternalServerError, Code: "failed", Message: err.Error()})
			return
		}
	}
	if err != nil {
		writeError(w, unauthorized("未登录"))
		return
	}
	writeData(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleSchools(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("q")
	schools, err := s.svc.SearchSchools(r.Context(), keyword, 80)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"schools": schools, "total": len(schools)})
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	features := []string{
		"manual-sign", "auto-plan", "multi-account", "photo",
		"password-login", "device-id",
	}
	if s.svc.Config().QuotaEnabled {
		features = append(features, "quota")
	}
	writeData(w, http.StatusOK, map[string]any{
		// appVersion 是产品版本（页脚/关于页展示），clientVersion 才是模拟的
		// 今日校园客户端版本；两者混用会把 9.9.24 当成 Qjrxy 的版本号。
		"appVersion":    buildinfo.Version,
		"clientVersion": cpdaily.AppVersion,
		"apiVersion":    "v1",
		"serverTime":    time.Now().Format(time.RFC3339),
		"tokenTTLDay":   s.tokenTTL,
		"quotaEnabled":  s.svc.Config().QuotaEnabled,
		"diagEnabled":   diag.Enabled(),
		"multiUser":     !s.svc.Config().SingleUser,
		// 局域网可访问地址：扫码授权需要手机用这些地址打开本机
		"lanUrls":  LANBaseURLs(s.svc.Config().Addr),
		"features": features,
	})
}

func (s *Server) issueSession(r *http.Request, userID int64) (string, error) {
	token := secure.NewToken()
	if err := s.svc.DB().CreateSession(r.Context(), token, userID, time.Duration(s.tokenTTL)*24*time.Hour); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Server) setTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: "qj_token", Value: token, Path: "/",
		MaxAge: s.tokenTTL * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}
