// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

// 管理员接口：运行时配置（诊断开关、识别服务参数）的查看与修改。
//
// 权限：登录之外还要求管理员——多用户模式下 role=admin，单用户模式
// （本机自用部署）直接视为管理员。所有变更写 settings 表并立即热生效，
// 同时在终端日志里留一条不含敏感值的审计记录。
package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"qjrxy/internal/diag"
	"qjrxy/internal/secure"
	"qjrxy/internal/service"
	"qjrxy/internal/vision"
)

// requireAdmin 包装管理接口。
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := requireUser(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		if !s.svc.Config().SingleUser && user.Role != "admin" {
			writeError(w, &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "需要管理员权限"})
			return
		}
		next(w, r)
	}
}

// adminVision 是识别服务的对外视图：key 永远不下发，只给掩码。
type adminVision struct {
	HasKey  bool   `json:"hasKey"`
	KeyMask string `json:"keyMask"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
}

type adminServer struct {
	MultiUser        bool   `json:"multiUser"`
	RegisterOpen     bool   `json:"registerOpen"`
	TLSInsecureHosts string `json:"tlsInsecureHosts"`
}

type adminPhotoPool struct {
	LimitBytes int64 `json:"limitBytes"`
	UsedBytes  int64 `json:"usedBytes"`
	PhotoCount int   `json:"photoCount"`
}

type adminConfig struct {
	DiagEnabled  bool           `json:"diagEnabled"`
	DiagKeepDays int            `json:"diagKeepDays"`
	Vision       adminVision    `json:"vision"`
	Server       adminServer    `json:"server"`
	PhotoPool    adminPhotoPool `json:"photoPool"`
}

type adminConfigUpdate struct {
	DiagEnabled   *bool   `json:"diagEnabled"`
	DiagKeepDays  *int    `json:"diagKeepDays"`
	VisionAPIKey  *string `json:"visionApiKey"`
	VisionBaseURL *string `json:"visionBaseURL"`
	VisionModel   *string `json:"visionModel"`
	MultiUser     *bool   `json:"multiUser"`
	// AdminPassword 在开启多用户时为当前账号设置登录密码。
	// 单用户模式的本地用户密码是进程内随机生成、从未展示的，
	// 不在这里设一个可用的密码，会话一旦丢失就无法再登录（永久锁死）。
	AdminPassword       *string `json:"adminPassword"`
	RegisterOpen        *bool   `json:"registerOpen"`
	TLSInsecureHosts    *string `json:"tlsInsecureHosts"`
	PhotoPoolLimitBytes *int64  `json:"photoPoolLimitBytes"`
}

// handleAdminGetConfig 返回当前运行时配置。
// 优先级：settings 表（admin 页改过的）> 环境变量 > 内置默认。
func (s *Server) handleAdminGetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, err := s.svc.DB().AllSettings(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	cfg := s.svc.Config()

	diagOn := os.Getenv("QINGJIN_DIAG") != "off"
	if v := settings["diag.enabled"]; v != "" {
		diagOn = v != "0"
	}
	keep := 7
	if v := settings["diag.keepDays"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			keep = n
		}
	}

	cur := vision.Current()
	if v := settings["vision.baseURL"]; v != "" {
		cur.BaseURL = v
	}
	if v := settings["vision.model"]; v != "" {
		cur.Model = v
	}
	key := cur.APIKey
	if enc := settings["vision.apiKey"]; enc != "" {
		if plain, derr := secure.Decrypt(cfg.SecretKey, enc); derr == nil {
			key = plain
		}
	}

	poolUsed, poolCount := s.svc.PhotoPoolUsage()
	writeData(w, http.StatusOK, adminConfig{
		DiagEnabled:  diagOn,
		DiagKeepDays: keep,
		Vision: adminVision{
			HasKey:  strings.TrimSpace(key) != "",
			KeyMask: maskKey(key),
			BaseURL: cur.BaseURL,
			Model:   cur.Model,
		},
		Server: adminServer{
			MultiUser:        !cfg.SingleUser,
			RegisterOpen:     cfg.RegisterOpen,
			TLSInsecureHosts: strings.Join(cfg.TLSInsecureHosts, ","),
		},
		PhotoPool: adminPhotoPool{
			LimitBytes: s.svc.PhotoPoolLimit(ctx),
			UsedBytes:  poolUsed,
			PhotoCount: poolCount,
		},
	})
}

// handleAdminSaveConfig 更新配置并热生效。
func (s *Server) handleAdminSaveConfig(w http.ResponseWriter, r *http.Request) {
	var in adminConfigUpdate
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	changed := []string{}

	if in.DiagEnabled != nil {
		v := "1"
		if !*in.DiagEnabled {
			v = "0"
		}
		if err := s.svc.DB().SetSetting(ctx, "diag.enabled", v); err != nil {
			writeError(w, err)
			return
		}
		diag.SetEnabled(*in.DiagEnabled)
		changed = append(changed, "诊断日志="+onOff(*in.DiagEnabled))
	}
	if in.DiagKeepDays != nil {
		days := *in.DiagKeepDays
		if days <= 0 || days > 365 {
			writeError(w, badRequest("保留天数需在 1-365 之间"))
			return
		}
		if err := s.svc.DB().SetSetting(ctx, "diag.keepDays", strconv.Itoa(days)); err != nil {
			writeError(w, err)
			return
		}
		diag.SetKeepDays(days)
		changed = append(changed, fmt.Sprintf("诊断保留=%d天", days))
	}
	if in.VisionAPIKey != nil && strings.TrimSpace(*in.VisionAPIKey) != "" {
		enc, err := secure.Encrypt(s.svc.Config().SecretKey, strings.TrimSpace(*in.VisionAPIKey))
		if err != nil {
			writeError(w, fmt.Errorf("密钥加密失败：%w", err))
			return
		}
		if err := s.svc.DB().SetSetting(ctx, "vision.apiKey", enc); err != nil {
			writeError(w, err)
			return
		}
		changed = append(changed, "识别APIKey=已更新")
	}
	if in.VisionBaseURL != nil {
		if err := s.svc.DB().SetSetting(ctx, "vision.baseURL", strings.TrimSpace(*in.VisionBaseURL)); err != nil {
			writeError(w, err)
			return
		}
		changed = append(changed, "识别BaseURL="+onOff(*in.VisionBaseURL != ""))
	}
	if in.VisionModel != nil {
		if err := s.svc.DB().SetSetting(ctx, "vision.model", strings.TrimSpace(*in.VisionModel)); err != nil {
			writeError(w, err)
			return
		}
		changed = append(changed, "识别Model="+*in.VisionModel)
	}

	user, _ := requireUser(r.Context())
	if in.MultiUser != nil {
		if *in.MultiUser {
			// 开多用户后鉴权切到密码登录；当前操作者必须是管理员，
			// 否则单用户随机密码会把人锁在外面。管理员自己提权是安全内聚的。
			if user.Role != "admin" {
				if err := s.svc.DB().UpdateUserRole(ctx, user.ID, "admin"); err != nil {
					writeError(w, err)
					return
				}
				user.Role = "admin"
				changed = append(changed, "当前用户已提升为管理员")
			}
			// 开多用户的同时必须给当前账号一个可用的登录密码：
			// 单用户本地用户的密码是进程内随机生成的，从没展示给任何人，
			// 不设的话会话一丢就永久锁死。
			if in.AdminPassword != nil && strings.TrimSpace(*in.AdminPassword) != "" {
				pwd := strings.TrimSpace(*in.AdminPassword)
				if len(pwd) < 6 {
					writeError(w, badRequest("管理员密码至少 6 位"))
					return
				}
				hash, perr := secure.HashPassword(pwd)
				if perr != nil {
					writeError(w, perr)
					return
				}
				if perr := s.svc.DB().UpdateUserPassword(ctx, user.ID, hash); perr != nil {
					writeError(w, perr)
					return
				}
				changed = append(changed, "当前账号已设置登录密码")
			}
			if err := s.svc.DB().SetSetting(ctx, "server.multiUser", "1"); err != nil {
				writeError(w, err)
				return
			}
			changed = append(changed, "多用户模式=开")
		} else {
			if err := s.svc.DB().SetSetting(ctx, "server.multiUser", "0"); err != nil {
				writeError(w, err)
				return
			}
			changed = append(changed, "多用户模式=关")
		}
	}
	if in.RegisterOpen != nil {
		v := "0"
		if *in.RegisterOpen {
			v = "1"
		}
		if err := s.svc.DB().SetSetting(ctx, "server.registerOpen", v); err != nil {
			writeError(w, err)
			return
		}
		changed = append(changed, "自助注册="+onOff(*in.RegisterOpen))
	}
	if in.TLSInsecureHosts != nil {
		hosts := strings.TrimSpace(*in.TLSInsecureHosts)
		if err := s.svc.DB().SetSetting(ctx, "server.tlsInsecureHosts", hosts); err != nil {
			writeError(w, err)
			return
		}
		changed = append(changed, "证书白名单="+onOff(hosts != ""))
	}
	if in.PhotoPoolLimitBytes != nil {
		limit := *in.PhotoPoolLimitBytes
		// 1MB 下限避免把池子设成永远无法上传；1TB 上限防止误填天文数字。
		if limit < 1<<20 || limit > 1<<40 {
			writeError(w, badRequest("照片池总容量需在 1MB-1TB 之间"))
			return
		}
		if err := s.svc.DB().SetSetting(ctx, service.PhotoPoolLimitKey, strconv.FormatInt(limit, 10)); err != nil {
			writeError(w, err)
			return
		}
		changed = append(changed, fmt.Sprintf("照片池上限=%dMB", limit>>20))
	}

	// 热生效：全部运行时配置从「settings > env」合并后重新注入。
	s.svc.ApplyRuntimeSettings()

	// 开多用户会把鉴权从「免登录」切成「密码登录」：给当前浏览器签发
	// 一个正式会话，避免保存完一刷新就被踢到登录页（单用户密码是随机占位）。
	var issuedToken string
	if in.MultiUser != nil && *in.MultiUser && user != nil {
		if token, terr := s.issueSession(r, user.ID); terr == nil {
			issuedToken = token
		}
	}

	if len(changed) > 0 {
		log.Printf("[admin] 配置变更：%s", strings.Join(changed, "，"))
	}
	writeData(w, http.StatusOK, map[string]string{"message": "已保存并生效", "token": issuedToken})
}

// maskKey 掩码：只留前 3 与后 4 位。
func maskKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:3] + "****" + key[len(key)-4:]
}

func onOff(b bool) string {
	if b {
		return "开"
	}
	return "关"
}
