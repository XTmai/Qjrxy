package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"qjrxy/internal/config"
	"qjrxy/internal/server"
	"qjrxy/internal/service"
	"qjrxy/internal/store"
)

type testEnv struct {
	srv   *httptest.Server
	token string
	db    *store.DB
	svc   *service.Service
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	cfg := &config.Config{
		Addr:         ":0",
		SecretKey:    bytes.Repeat([]byte{7}, 32),
		QuotaEnabled: true, // 兑换等用例依赖额度体系
		RegisterOpen: true, // 多用户用例需要在同一实例内注册多个账号
	}
	return newEnvWith(t, cfg)
}

func newEnvWith(t *testing.T, cfg *config.Config) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg.DataDir = dir
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := service.New(cfg, db)
	handler := server.New(svc).Handler(nil)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	env := &testEnv{srv: srv, db: db, svc: svc}
	if cfg.RegisterOpen {
		// 首个注册用户自动成为管理员（admin 接口的权限测试依赖这一点）
		env.register(t, "apiuser", "secret123")
	}
	return env
}

func (e *testEnv) do(t *testing.T, method, path string, body any, token string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求 %s %s 失败：%v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func (e *testEnv) register(t *testing.T, username, password string) {
	t.Helper()
	status, body := e.do(t, http.MethodPost, "/api/auth/register",
		map[string]string{"username": username, "password": password, "nickname": "测试"}, "")
	if status != http.StatusCreated {
		t.Fatalf("注册失败：%d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("注册未返回 token：%v", body)
	}
	e.token = token
}

// TestHealthAndSelfTest 校验公开接口可用。
func TestHealthAndSelfTest(t *testing.T) {
	env := newEnv(t)

	status, body := env.do(t, http.MethodGet, "/api/health", nil, "")
	if status != http.StatusOK || body["ok"] != true {
		t.Errorf("健康检查异常：%d %v", status, body)
	}

	status, body = env.do(t, http.MethodGet, "/api/selftest", nil, "")
	if status != http.StatusOK {
		t.Fatalf("自检失败：%d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["crypto"] != "ok" || data["database"] != "ok" {
		t.Errorf("自检结果异常：%v", data)
	}
}

// TestAuthFlow 校验注册、登录、me、登出。
func TestAuthFlow(t *testing.T) {
	env := newEnv(t)

	status, body := env.do(t, http.MethodGet, "/api/auth/me", nil, env.token)
	if status != http.StatusOK {
		t.Fatalf("me 失败：%d %v", status, body)
	}

	// 错误密码
	status, _ = env.do(t, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "apiuser", "password": "wrong"}, "")
	if status != http.StatusBadRequest {
		t.Errorf("错误密码应返回 400，得到 %d", status)
	}

	// 正确密码
	status, body = env.do(t, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "apiuser", "password": "secret123"}, "")
	if status != http.StatusOK {
		t.Fatalf("登录失败：%d %v", status, body)
	}

	// 登出后 token 失效
	if status, _ := env.do(t, http.MethodPost, "/api/auth/logout", map[string]any{}, env.token); status != http.StatusOK {
		t.Error("登出失败")
	}
	if status, _ := env.do(t, http.MethodGet, "/api/auth/me", nil, env.token); status != http.StatusUnauthorized {
		t.Errorf("登出后应返回 401，得到 %d", status)
	}
}

// TestUnauthorizedAccess 校验未登录无法访问业务接口。
func TestUnauthorizedAccess(t *testing.T) {
	env := newEnv(t)
	for _, path := range []string{
		"/api/accounts", "/api/tasks", "/api/plans", "/api/logs", "/api/dashboard",
	} {
		status, body := env.do(t, http.MethodGet, path, nil, "")
		if status != http.StatusUnauthorized {
			t.Errorf("%s 未登录应返回 401，得到 %d", path, status)
		}
		if body["ok"] != false {
			t.Errorf("%s 错误响应格式不符：%v", path, body)
		}
	}
	// 非法 token 同样拒绝
	if status, _ := env.do(t, http.MethodGet, "/api/accounts", nil, "invalid-token"); status != http.StatusUnauthorized {
		t.Errorf("非法 token 应返回 401，得到 %d", status)
	}
}

// TestDashboardEmpty 校验空状态工作台。
func TestDashboardEmpty(t *testing.T) {
	env := newEnv(t)
	status, body := env.do(t, http.MethodGet, "/api/dashboard", nil, env.token)
	if status != http.StatusOK {
		t.Fatalf("工作台失败：%d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	stats, _ := data["stats"].(map[string]any)
	if stats["todayTotal"] != float64(0) {
		t.Errorf("初始今日执行应为 0：%v", stats)
	}
	if accounts, _ := data["accounts"].([]any); len(accounts) != 0 {
		t.Errorf("初始账号列表应为空：%v", accounts)
	}
	user, _ := data["user"].(map[string]any)
	if user["quota"] != float64(50) {
		t.Errorf("初始额度应为 50：%v", user["quota"])
	}
}

// TestValidation 校验入参校验。
func TestValidation(t *testing.T) {
	env := newEnv(t)

	// 缺少学校
	status, body := env.do(t, http.MethodPost, "/api/accounts",
		map[string]any{"username": "2023"}, env.token)
	if status != http.StatusBadRequest {
		t.Errorf("缺少学校应返回 400，得到 %d", status)
	}

	// 缺少账号 ID
	status, _ = env.do(t, http.MethodPost, "/api/tasks/sync", map[string]any{}, env.token)
	if status != http.StatusBadRequest {
		t.Errorf("缺少 accountId 应返回 400，得到 %d", status)
	}

	// 不存在的账号
	status, _ = env.do(t, http.MethodPost, "/api/tasks/sync", map[string]any{"accountId": 999}, env.token)
	if status == http.StatusOK {
		t.Error("不存在的账号不应返回成功")
	}

	// 不存在的路由返回 JSON 404 或 405
	status, _ = env.do(t, http.MethodGet, "/api/nope", nil, env.token)
	if status == http.StatusOK {
		t.Errorf("未知路由不应返回 200")
	}
	_ = body
}

// TestRedeemQuota 校验卡密兑换。
func TestRedeemQuota(t *testing.T) {
	env := newEnv(t)

	// 卡密必须由运维预置入库，面额以库中的发卡记录为准。
	if err := env.db.CreateRedeemCode(context.Background(), "QJ-A1B2-C3D4", 30); err != nil {
		t.Fatalf("预置卡密失败：%v", err)
	}

	status, body := env.do(t, http.MethodPost, "/api/quota/redeem",
		map[string]string{"code": "QJ-A1B2-C3D4"}, env.token)
	if status != http.StatusOK {
		t.Fatalf("兑换失败：%d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["granted"] != float64(30) {
		t.Errorf("应兑换 30 次，得到 %v", data["granted"])
	}

	// 同一张卡密只能兑换一次。
	status, _ = env.do(t, http.MethodPost, "/api/quota/redeem",
		map[string]string{"code": "QJ-A1B2-C3D4"}, env.token)
	if status != http.StatusBadRequest {
		t.Errorf("重复兑换应被拒绝，得到 %d", status)
	}

	// 格式合法但未入库的卡密必须失败（回归：早期实现会白送 30 次）。
	status, _ = env.do(t, http.MethodPost, "/api/quota/redeem",
		map[string]string{"code": "QJ-0000-0000"}, env.token)
	if status != http.StatusBadRequest {
		t.Errorf("未入库的卡密应被拒绝，得到 %d", status)
	}

	for _, bad := range []string{"", "ABC", "QJ-XX", "QJ-GGGG-HHHH"} {
		status, _ := env.do(t, http.MethodPost, "/api/quota/redeem",
			map[string]string{"code": bad}, env.token)
		if status != http.StatusBadRequest {
			t.Errorf("非法卡密 %q 应返回 400，得到 %d", bad, status)
		}
	}
}

// TestChangePassword 校验改密流程。
func TestChangePassword(t *testing.T) {
	env := newEnv(t)

	status, _ := env.do(t, http.MethodPost, "/api/auth/password",
		map[string]string{"oldPassword": "wrong", "newPassword": "newpass123"}, env.token)
	if status != http.StatusBadRequest {
		t.Errorf("原密码错误应返回 400，得到 %d", status)
	}

	status, _ = env.do(t, http.MethodPost, "/api/auth/password",
		map[string]string{"oldPassword": "secret123", "newPassword": "newpass123"}, env.token)
	if status != http.StatusOK {
		t.Fatalf("改密失败：%d", status)
	}

	status, _ = env.do(t, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "apiuser", "password": "newpass123"}, "")
	if status != http.StatusOK {
		t.Errorf("新密码应可登录，得到 %d", status)
	}
}

// TestDeviceTicketRequiresOwnership 校验设备票据不能作用于他人账号。
func TestDeviceTicketRequiresOwnership(t *testing.T) {
	env := newEnv(t)

	// 另一个用户
	other := &testEnv{srv: env.srv}
	other.register(t, "otheruser", "secret123")

	status, _ := other.do(t, http.MethodPost, "/api/accounts/1/device-ticket", map[string]any{}, other.token)
	if status == http.StatusOK {
		t.Error("不能为不存在的账号生成票据")
	}
}

// TestDeviceSubmitRequiresValidTicket 校验设备回填必须持有效票据。
func TestDeviceSubmitRequiresValidTicket(t *testing.T) {
	env := newEnv(t)

	status, body := env.do(t, http.MethodPost, "/api/device/submit",
		map[string]string{"ticket": "forged", "deviceId": "abc"}, "")
	if status != http.StatusBadRequest {
		t.Fatalf("伪造票据应返回 400，得到 %d %v", status, body)
	}
	errObj, _ := body["error"].(map[string]any)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "票据") {
		t.Errorf("错误信息应说明票据问题：%q", msg)
	}
}

// TestEnvelopeShape 校验所有响应都符合统一信封。
func TestEnvelopeShape(t *testing.T) {
	env := newEnv(t)

	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+env.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("响应应为 JSON：%s", ct)
	}
	raw, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		t.Errorf("响应应为 JSON 对象：%s", raw)
	}
}

// TestRegisterClosedByDefault 校验默认配置下，注册口在首个账号创建后自动关闭。
//
// 公网实例被陌生人注册占用的根源就在这里：早期实现不设任何门，
// 任何人扫到端口都能注册并把账号挂到别人的服务器上。
func TestRegisterClosedByDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Addr:      ":0",
		DataDir:   dir,
		SecretKey: bytes.Repeat([]byte{9}, 32),
		// 故意不设置 RegisterOpen：走默认的「仅首次初始化可注册」
	}
	db, err := store.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := service.New(cfg, db)
	srv := httptest.NewServer(server.New(svc).Handler(nil))
	t.Cleanup(srv.Close)
	env := &testEnv{srv: srv, db: db, svc: svc}

	// 库里没有用户：允许创建第一个账号
	env.register(t, "firstuser", "secret123")

	status, body := env.do(t, http.MethodPost, "/api/auth/register",
		map[string]string{"username": "intruder", "password": "secret123"}, "")
	if status != http.StatusForbidden {
		t.Fatalf("第二个账号应被拒绝注册，得到 %d %v", status, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "register_closed" {
		t.Errorf("错误码应为 register_closed，得到 %v", errObj)
	}
}

// TestLoginRateLimited 校验登录接口对同一账号的连续尝试做限流。
func TestLoginRateLimited(t *testing.T) {
	env := newEnv(t)

	// 限流窗口内允许 5 次尝试，第 6 次起返回 429。
	var last int
	for i := 0; i < 7; i++ {
		last, _ = env.do(t, http.MethodPost, "/api/auth/login",
			map[string]string{"username": "apiuser", "password": "wrong-password"}, "")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("连续失败尝试应触发 429，得到 %d", last)
	}

	// 限流不应影响其他账号的正常登录。
	env.register(t, "otheruser", "secret123")
	status, body := env.do(t, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "otheruser", "password": "secret123"}, "")
	if status != http.StatusOK {
		t.Fatalf("其他账号登录应正常，得到 %d %v", status, body)
	}
}

// TestChangePasswordRevokesOtherSessions 校验改密后其他会话立即失效。
func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	env := newEnv(t)

	// 模拟另一台设备登录，拿到第二个 token。
	_, body := env.do(t, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "apiuser", "password": "secret123"}, "")
	data, _ := body["data"].(map[string]any)
	otherToken, _ := data["token"].(string)
	if otherToken == "" {
		t.Fatalf("登录未返回 token：%v", body)
	}

	status, _ := env.do(t, http.MethodPost, "/api/auth/password",
		map[string]string{"oldPassword": "secret123", "newPassword": "newpass123"}, env.token)
	if status != http.StatusOK {
		t.Fatalf("改密失败：%d", status)
	}

	// 改密前的另一个会话应失效。
	status, _ = env.do(t, http.MethodGet, "/api/auth/me", nil, otherToken)
	if status != http.StatusUnauthorized {
		t.Errorf("其他设备会话应在改密后失效，得到 %d", status)
	}

	// 发起改密的当前会话保持可用。
	status, _ = env.do(t, http.MethodGet, "/api/auth/me", nil, env.token)
	if status != http.StatusOK {
		t.Errorf("当前会话应继续有效，得到 %d", status)
	}
}

// newSingleEnv 构造单用户模式实例（cfg.SingleUser=true，模拟本地 exe 直开）。
func newSingleEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Addr:       ":0",
		DataDir:    dir,
		SecretKey:  bytes.Repeat([]byte{9}, 32),
		SingleUser: true,
	}
	db, err := store.Open(filepath.Join(dir, "single.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := service.New(cfg, db)
	srv := httptest.NewServer(server.New(svc).Handler(nil))
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, db: db, svc: svc}
}

// TestSingleUserMode 校验单用户模式：无 token 直用、注册口关闭、老库用户无缝复用。
func TestSingleUserMode(t *testing.T) {
	env := newSingleEnv(t)

	// 无 token 的 /auth/me 应自动创建并返回本地用户
	status, body := env.do(t, http.MethodGet, "/api/auth/me", nil, "")
	if status != http.StatusOK {
		t.Fatalf("单用户模式 /auth/me 应 200，得到 %d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	user, _ := data["user"].(map[string]any)
	if user == nil || user["username"] != "admin" {
		t.Fatalf("应自动创建本地用户 admin，得到 %v", body)
	}

	// 业务接口同样无 token 可用
	if status, _ := env.do(t, http.MethodGet, "/api/accounts", nil, ""); status != http.StatusOK {
		t.Errorf("单用户模式 /api/accounts 应 200，得到 %d", status)
	}
	if status, _ := env.do(t, http.MethodGet, "/api/dashboard", nil, ""); status != http.StatusOK {
		t.Errorf("单用户模式 /api/dashboard 应 200，得到 %d", status)
	}

	// 注册口关闭（code=single_user）
	status, body = env.do(t, http.MethodPost, "/api/auth/register",
		map[string]string{"username": "intruder", "password": "hack9999"}, "")
	if status != http.StatusForbidden {
		t.Errorf("单用户模式注册应 403，得到 %d", status)
	}
	if errObj, _ := body["error"].(map[string]any); errObj != nil && errObj["code"] != "single_user" {
		t.Errorf("错误码应为 single_user，得到 %v", errObj)
	}

	// 老库已有用户：新实例应复用第一个用户而不是再建 admin
	//（用户须在单用户实例首次访问前就存在，才能测出「复用」路径）
	dir2 := t.TempDir()
	db2, err := store.Open(filepath.Join(dir2, "legacy.db"))
	if err != nil {
		t.Fatalf("打开老库失败：%v", err)
	}
	defer db2.Close()
	if _, err := db2.CreateUser(context.Background(), "legacyuser", "老用户", "hash", 50); err != nil {
		t.Fatalf("预置老用户失败：%v", err)
	}
	svc2 := service.New(&config.Config{
		Addr: ":0", DataDir: dir2,
		SecretKey: bytes.Repeat([]byte{8}, 32), SingleUser: true,
	}, db2)
	srv2 := httptest.NewServer(server.New(svc2).Handler(nil))
	defer srv2.Close()

	req, _ := http.NewRequest(http.MethodGet, srv2.URL+"/api/auth/me", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("复用老用户时 /auth/me 应 200，得到 %d %s", resp.StatusCode, raw)
	}
	outData, _ := out["data"].(map[string]any)
	if u, _ := outData["user"].(map[string]any); u == nil || u["username"] != "legacyuser" {
		t.Errorf("应复用已有用户 legacyuser，得到 %v", out)
	}
}
