package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"qingjin/internal/config"
	"qingjin/internal/cpdaily"
	"qingjin/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.User) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, SecretKey: make([]byte, 32)}
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := New(cfg, db)
	u, err := svc.Register(context.Background(), "tester", "pass123456", "测试")
	if err != nil {
		t.Fatalf("注册失败：%v", err)
	}
	return svc, u
}

func newTestAccount(t *testing.T, svc *Service, userID int64) *store.Account {
	t.Helper()
	acc, err := svc.DB().CreateAccount(context.Background(), &store.Account{
		UserID: userID, SchoolName: "测试大学", Username: "2023001", Host: "t.campusphere.net",
	})
	if err != nil {
		t.Fatalf("建账号失败：%v", err)
	}
	return acc
}

// TestDeviceTicketPolling 校验「扫码写入 → 轮询读取 → 落库」完整流程。
func TestDeviceTicketPolling(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	ticket, _, err := svc.CreateDeviceTicket(ctx, user.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 扫码前：pending
	pending, info, err := svc.PollDeviceTicket(ticket)
	if err != nil {
		t.Fatalf("轮询不应报错：%v", err)
	}
	if !pending || info != nil {
		t.Error("未扫码时应返回 pending=true")
	}

	// 扫码页提交
	if err := svc.SubmitDeviceInfo(ticket, DeviceInfo{
		DeviceID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		UUID:     "11111111-2222-3333-4444-555555555555",
		DeviceModel: "V2405A", SystemVersion: "16",
		BuildID: "AP2A.240705.005", SystemName: "android",
	}); err != nil {
		t.Fatalf("提交失败：%v", err)
	}

	// 扫码后：拿到信息
	pending, info, err = svc.PollDeviceTicket(ticket)
	if err != nil {
		t.Fatal(err)
	}
	if pending || info == nil {
		t.Fatal("扫码后应返回结果")
	}
	if info.DeviceID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("deviceId 不符：%s", info.DeviceID)
	}
	if info.Platform != "android" {
		t.Errorf("平台归一化错误：%s", info.Platform)
	}
	if info.CollectedAt == "" {
		t.Error("应记录采集时间")
	}

	// 落库
	got, err := svc.CollectDeviceInfo(ctx, user.ID, ticket)
	if err != nil {
		t.Fatalf("落库失败：%v", err)
	}
	if got.DeviceID != info.DeviceID || got.DeviceModel != "V2405A" || got.SystemVersion != "16" {
		t.Errorf("账号未正确回填：%+v", got)
	}

	// 票据已消费
	if _, _, err := svc.PollDeviceTicket(ticket); err == nil {
		t.Error("票据消费后轮询应失败")
	}
}

// TestDeviceTicketOwnership 校验票据不能跨用户使用。
func TestDeviceTicketOwnership(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	ticket, _, _ := svc.CreateDeviceTicket(ctx, user.ID, acc.ID)
	_ = svc.SubmitDeviceInfo(ticket, DeviceInfo{DeviceID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"})

	other, err := svc.Register(ctx, "other", "pass123456", "别人")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CollectDeviceInfo(ctx, other.ID, ticket); err == nil {
		t.Error("其他用户不应能消费该票据")
	}
}

// TestDeviceIOSTicket 校验 iOS 平台识别。
func TestDeviceIOSTicket(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	ticket, _, _ := svc.CreateDeviceTicket(ctx, user.ID, acc.ID)
	err := svc.SubmitDeviceInfo(ticket, DeviceInfo{
		DeviceID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		DeviceModel: "iPhone 16 Pro", SystemVersion: "18.2",
		SystemName: "iOS", WisDeviceID: "enc.app.aeb.v1.xxx",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.CollectDeviceInfo(ctx, user.ID, ticket)
	if err != nil {
		t.Fatal(err)
	}
	if got.Platform != "ios" {
		t.Errorf("SystemName=iOS 应归一化为 ios，得到 %s", got.Platform)
	}
	if got.DeviceModel != "iPhone 16 Pro" {
		t.Errorf("机型未回填：%s", got.DeviceModel)
	}
}

// TestDeviceTicketInvalid 校验无效票据与空设备 ID。
func TestDeviceTicketInvalid(t *testing.T) {
	svc, user := newTestService(t)
	ctx := context.Background()

	if _, _, err := svc.PollDeviceTicket("nonexistent"); err == nil {
		t.Error("无效票据轮询应报错")
	}
	if err := svc.SubmitDeviceInfo("nonexistent", DeviceInfo{DeviceID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}); err == nil {
		t.Error("无效票据提交应报错")
	}

	acc := newTestAccount(t, svc, user.ID)
	ticket, _, _ := svc.CreateDeviceTicket(ctx, user.ID, acc.ID)
	if err := svc.SubmitDeviceInfo(ticket, DeviceInfo{DeviceID: "  "}); err == nil {
		t.Error("空 deviceId 应被拒绝")
	}
}

// TestRunRequiresDeviceID 校验未绑定设备时拒绝提交。
// 目的：防止用空 deviceId 去撞服务端，触发「更换手机」风控。
func TestRunRequiresDeviceID(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	// 手工塞一个假 Cookie，绕过"未登录"检查，单独验证设备检查
	acc.Cookies = `{"MOD_AUTH_CAS":"x"}`
	if err := svc.DB().UpdateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}

	_, err := svc.RunTask(ctx, RunOptions{UserID: user.ID, AccountID: acc.ID})
	if err == nil {
		t.Fatal("未绑定设备时应拒绝执行")
	}
	if !strings.Contains(err.Error(), "设备") {
		t.Errorf("错误信息应提到设备：%v", err)
	}
}

// TestOnlyAttendanceAllowed 校验只允许执行查寝任务。
func TestOnlyAttendanceAllowed(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	// 补齐登录态与设备，让守卫走到"任务类型"这一层
	acc.Cookies = `{"MOD_AUTH_CAS":"x"}`
	acc.DeviceID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	if err := svc.DB().UpdateAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}

	_, err := svc.RunTask(ctx, RunOptions{UserID: user.ID, AccountID: acc.ID, Kind: "sign"})
	if err == nil {
		t.Fatal("sign 类型应被拒绝")
	}
	if !strings.Contains(err.Error(), "查寝") {
		t.Errorf("错误信息应说明只支持查寝：%v", err)
	}
}

// TestUserAgentPreference 校验扫码回传的 UA 优先于拼接。
func TestUserAgentPreference(t *testing.T) {
	raw := "Mozilla/5.0 (Linux; Android 10; TEL-AN10 Build/HONORTEL-AN10; wv) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Version/4.0 Chrome/120.0.6099.230 Mobile Safari/537.36 " +
		"okhttp/4.12.0 cpdaily/9.9.24 wisedu/9.9.24"
	d := cpdaily.DeviceProfile{
		RawUserAgent: raw,
		DeviceModel:  "别的机型", SystemVersion: "99",
	}
	if got := d.UserAgent(); got != raw {
		t.Errorf("应原样使用扫码 UA，得到：%s", got)
	}

	// 没有原始 UA 时回退到拼接
	d2 := cpdaily.DeviceProfile{DeviceModel: "TEL-AN10", SystemVersion: "10"}
	if got := d2.UserAgent(); !strings.Contains(got, "TEL-AN10") {
		t.Errorf("无原始 UA 时应拼接，得到：%s", got)
	}
}

// TestCheckTaskWindow 校验任务时间窗口拦截。
// 回归：服务端在任务未开始时返回 code=0 但 signedStuInfo 为空，
// 看起来像成功，因此在客户端提前拦住未到时间的任务。
func TestCheckTaskWindow(t *testing.T) {
	cases := []struct {
		status  string
		wantErr bool
		keyword string
	}{
		{"2", true, "还没到签到时间"},
		{"3", true, "已过签到时间"},
		{"4", true, "已签到"},
		{"0", false, ""},
		{"1", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		task := &store.Task{TaskName: "测试查寝", SignStatus: c.status}
		err := checkTaskWindow(task)
		if c.wantErr && err == nil {
			t.Errorf("signStatus=%q 应被拦截", c.status)
			continue
		}
		if !c.wantErr && err != nil {
			t.Errorf("signStatus=%q 不应拦截：%v", c.status, err)
			continue
		}
		if c.wantErr && !strings.Contains(err.Error(), c.keyword) {
			t.Errorf("signStatus=%q 提示应含 %q，得到 %v", c.status, c.keyword, err)
		}
	}
}

// TestUpdateAccountFields 校验编辑账号时各字段都会真正写入。
// 回归：学号、学校之前没有在 UpdateAccount 里赋值，改完只提示成功但值不变。
func TestUpdateAccountFields(t *testing.T) {
	svc, user := newTestService(t)
	ctx := context.Background()
	acc := newTestAccount(t, svc, user.ID)

	// 改学号
	updated, err := svc.UpdateAccount(ctx, user.ID, acc.ID, AccountInput{
		Username: "new-student-id",
	})
	if err != nil {
		t.Fatalf("更新学号失败：%v", err)
	}
	if updated.Username != "new-student-id" {
		t.Errorf("学号未更新，仍为 %q", updated.Username)
	}

	// 改设备信息
	updated, err = svc.UpdateAccount(ctx, user.ID, acc.ID, AccountInput{
		DeviceID:      "80f92ef6-c8be-476e-86c6-718df6255c97",
		DeviceModel:   "TEL-AN10",
		SystemVersion: "10",
		BuildID:       "HONORTEL-AN10",
		AppVersion:    "9.9.24",
		UserAgent:     "test-ua",
		Platform:      "android",
	})
	if err != nil {
		t.Fatalf("更新设备失败：%v", err)
	}
	if updated.DeviceID != "80f92ef6-c8be-476e-86c6-718df6255c97" {
		t.Errorf("设备 ID 未更新：%q", updated.DeviceID)
	}
	if updated.DeviceModel != "TEL-AN10" || updated.SystemVersion != "10" {
		t.Errorf("机型/版本未更新：%q %q", updated.DeviceModel, updated.SystemVersion)
	}
	if updated.BuildID != "HONORTEL-AN10" || updated.AppVersion != "9.9.24" {
		t.Errorf("构建号/App版本未更新：%q %q", updated.BuildID, updated.AppVersion)
	}
	if updated.UserAgent != "test-ua" {
		t.Errorf("UA 未更新：%q", updated.UserAgent)
	}

	// 改坐标
	updated, err = svc.UpdateAccount(ctx, user.ID, acc.ID, AccountInput{
		Lon: 111.2195, Lat: 30.7101, Address: "三峡大学科技学院新校区",
	})
	if err != nil {
		t.Fatalf("更新位置失败：%v", err)
	}
	if updated.Lon != 111.2195 || updated.Lat != 30.7101 {
		t.Errorf("坐标未更新：%v %v", updated.Lon, updated.Lat)
	}
	if updated.Address != "三峡大学科技学院新校区" {
		t.Errorf("地址未更新：%q", updated.Address)
	}

	// 重新读库确认已持久化
	reloaded, err := svc.DB().AccountByID(ctx, user.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Username != "new-student-id" || reloaded.DeviceID != "80f92ef6-c8be-476e-86c6-718df6255c97" {
		t.Errorf("数据未持久化：user=%q device=%q", reloaded.Username, reloaded.DeviceID)
	}
}
