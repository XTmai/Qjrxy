package server

import (
	"net/http"
	"time"

	"qingjin/internal/cpdaily"
	"qingjin/internal/secure"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
		"name":   "青衿 · 今日校园签到与查寝",
	})
}

// handleSelfTest 校验加密链路与数据库连通性，便于部署后自检。
func (s *Server) handleSelfTest(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{"crypto": "ok", "database": "ok", "appVersion": cpdaily.AppVersion}
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
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
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
	user, token, err := s.svc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
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
	writeData(w, http.StatusOK, map[string]any{
		"appVersion":  cpdaily.AppVersion,
		"apiVersion":  "v1",
		"serverTime":  time.Now().Format(time.RFC3339),
		"tokenTTLDay": s.tokenTTL,
		"features": []string{
			"manual-sign", "auto-plan", "multi-account", "photo",
			"cookie-import", "password-login", "device-id",
		},
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
