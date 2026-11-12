package server_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"qjrxy/internal/config"
	"qjrxy/internal/service"
)

// 管理员接口的权限三态：role=admin 放行、role=user 拒绝、单用户模式放行。
// 环境里第一个注册的用户自动成为管理员（Register 的首管理员规则）。

func TestAdminFirstUserBecomesAdmin(t *testing.T) {
	env := newEnv(t)
	user, err := env.db.UserByUsername(context.Background(), "apiuser")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "admin" {
		t.Fatalf("首个注册用户应为 admin，得到 %q", user.Role)
	}
}

func TestAdminConfigForbiddenForUserRole(t *testing.T) {
	env := newEnv(t)
	// 第二个注册的用户是普通角色
	env.register(t, "plainuser", "secret123")
	// register 会覆盖 env.token 为最新用户（plainuser，role=user）
	status, body := env.do(t, "GET", "/api/admin/config", nil, env.token)
	if status != 403 {
		t.Fatalf("普通用户访问管理员接口应 403，得到 %d %v", status, body)
	}
	// 管理员（首个用户）仍然放行
	adminUser, err := env.db.UserByUsername(context.Background(), "apiuser")
	if err != nil {
		t.Fatal(err)
	}
	_ = adminUser
}

func TestAdminConfigAllowedForAdmin(t *testing.T) {
	env := newEnv(t) // env.token 属于首个用户（admin）
	status, body := env.do(t, "GET", "/api/admin/config", nil, env.token)
	if status != 200 {
		t.Fatalf("管理员读取配置应 200，得到 %d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["diagEnabled"] == nil {
		t.Fatalf("响应缺少 diagEnabled：%v", body)
	}

	// 修改诊断开关并确认热生效
	newOn := data["diagEnabled"].(bool) == false
	status, body = env.do(t, "POST", "/api/admin/config",
		map[string]any{"diagEnabled": newOn}, env.token)
	if status != 200 {
		t.Fatalf("管理员保存配置应 200，得到 %d %v", status, body)
	}
	// 再读：值已变化，且 settings 表已落
	status, body = env.do(t, "GET", "/api/admin/config", nil, env.token)
	if status != 200 {
		t.Fatalf("回读配置失败：%d", status)
	}
	data, _ = body["data"].(map[string]any)
	if data["diagEnabled"].(bool) != newOn {
		t.Fatalf("诊断开关未生效：期望 %v", newOn)
	}
	v, err := env.db.GetSetting(context.Background(), "diag.enabled")
	if err != nil {
		t.Fatal(err)
	}
	want := "1"
	if !newOn {
		want = "0"
	}
	if v != want {
		t.Fatalf("settings 表 diag.enabled = %q，期望 %q", v, want)
	}

	// API key 应加密落库：库里不存在明文
	if _, body = env.do(t, "POST", "/api/admin/config",
		map[string]any{"visionApiKey": "sk-test-abcdef123456"}, env.token); status != 200 {
		t.Fatalf("保存 API key 失败：%d %v", status, body)
	}
	enc, err := env.db.GetSetting(context.Background(), "vision.apiKey")
	if err != nil || enc == "" {
		t.Fatalf("vision.apiKey 未落库：%q, err=%v", enc, err)
	}
	if strings.Contains(enc, "sk-test") {
		t.Fatalf("API key 疑似明文落库：%q", enc)
	}
}

func TestAdminConfigSingleUserAllowed(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Addr:       ":0",
		DataDir:    dir,
		SecretKey:  bytes.Repeat([]byte{9}, 32),
		SingleUser: true,
	}
	env := newEnvWith(t, cfg)
	// 单用户模式不带 token：withUser 自动落到本地用户，requireAdmin 放行
	status, body := env.do(t, "GET", "/api/admin/config", nil, "")
	if status != 200 {
		t.Fatalf("单用户模式访问管理员接口应 200，得到 %d %v", status, body)
	}
}

// TestAdminPhotoPoolLimit 校验管理员可设置照片池全局容量，并立即回读。
func TestAdminPhotoPoolLimit(t *testing.T) {
	env := newEnv(t)
	status, body := env.do(t, "GET", "/api/admin/config", nil, env.token)
	if status != http.StatusOK {
		t.Fatalf("读取配置失败：%d", status)
	}
	data, _ := body["data"].(map[string]any)
	pool, _ := data["photoPool"].(map[string]any)
	if pool == nil || pool["limitBytes"].(float64) != float64(service.DefaultPhotoPoolLimitBytes) {
		t.Fatalf("默认照片池上限应为 1GB，得到 %v", pool)
	}

	limit := int64(2 << 30)
	status, body = env.do(t, "POST", "/api/admin/config",
		map[string]any{"photoPoolLimitBytes": limit}, env.token)
	if status != http.StatusOK {
		t.Fatalf("保存上限失败：%d %v", status, body)
	}
	_, body = env.do(t, "GET", "/api/admin/config", nil, env.token)
	data, _ = body["data"].(map[string]any)
	pool, _ = data["photoPool"].(map[string]any)
	if pool["limitBytes"].(float64) != float64(limit) {
		t.Errorf("上限未生效：%v", pool)
	}
	if v, _ := env.db.GetSetting(context.Background(), service.PhotoPoolLimitKey); v != "2147483648" {
		t.Errorf("settings 未落库：%q", v)
	}

	status, _ = env.do(t, "POST", "/api/admin/config",
		map[string]any{"photoPoolLimitBytes": 0}, env.token)
	if status != http.StatusBadRequest {
		t.Errorf("非法上限应 400，得到 %d", status)
	}
}
