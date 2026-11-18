// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"qjrxy/internal/config"
	"qjrxy/internal/cpdaily"
	"qjrxy/internal/store"
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
		DeviceID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		UUID:        "11111111-2222-3333-4444-555555555555",
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
		DeviceID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
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

// TestNoRunnableTaskMessage 校验无任务时的提示能区分「未到时间」与「已完成」。
//
// 回归：调度器曾在任务未到时间时也挑去提交，被服务端拒为「查寝未开始」，
// 且该失败还会写死当天的执行机会。
func TestNoRunnableTaskMessage(t *testing.T) {
	attendance := func(signStatus string) store.Task {
		return store.Task{Kind: "attendance", SignStatus: signStatus}
	}

	cases := []struct {
		name  string
		tasks []store.Task
		want  string
	}{
		{"未到时间", []store.Task{attendance("2")}, "还未到签到时间"},
		{"已过期", []store.Task{attendance("3")}, "已过签到时间"},
		{"已签到", []store.Task{attendance("4")}, "已完成"},
		{"无任务", nil, "没有待处理"},
	}
	for _, c := range cases {
		got := noRunnableTaskMessage(c.tasks)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s：期望包含 %q，得到 %q", c.name, c.want, got)
		}
	}

	// 只统计查寝任务：签到任务不应影响提示
	got := noRunnableTaskMessage([]store.Task{{Kind: "sign", SignStatus: "2"}})
	if !strings.Contains(got, "没有待处理") {
		t.Errorf("非查寝任务不应计入，得到 %q", got)
	}
}

// TestProbeAccountWithoutCredentials 校验无密码账号不会被误判为可自动恢复。
//
// 回归：探测逻辑若对无密码账号也尝试自动登录，会反复失败并污染登录态。
func TestProbeAccountWithoutCredentials(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	res := svc.ProbeAccount(ctx, acc)
	if res.Status != "expired" {
		t.Errorf("无密码账号应标记为 expired，得到 %q", res.Status)
	}
	if res.Relogged {
		t.Error("无密码账号不应触发自动重登")
	}
	if !strings.Contains(res.Message, "手动") {
		t.Errorf("应提示手动登录，得到 %q", res.Message)
	}
	// 探测结果应落库
	updated, err := svc.DB().AccountByID(ctx, user.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastProbeAt == "" {
		t.Error("探测时间应写入数据库")
	}
	if updated.LoginStatus != "expired" {
		t.Errorf("登录态应为 expired，得到 %q", updated.LoginStatus)
	}
}

// TestCheckTaskWindowByClock 校验 signStatus 缺失时按任务时段判断窗口。
//
// 回归：某任务 signDate=2026-11-03、beginClock=21:00、endClock=23:00，
// signStatus 为空，曾在 16:17 显示为「可执行」，提交后被服务端拒绝为「查寝未开始」。
func TestCheckTaskWindowByClock(t *testing.T) {
	task := &store.Task{
		TaskName:   "2024级学生查寝打卡",
		SignDate:   "2026-11-03",
		BeginClock: "21:00",
		EndClock:   "23:00",
		SignStatus: "", // 学校未下发状态
	}

	// 16:17 —— 还没到时间
	before := time.Date(2026, 11, 3, 16, 17, 0, 0, time.Local)
	if err := checkTaskWindowByClock(task, before); err == nil {
		t.Error("16:17 应判定为未到签到时间")
	} else if !strings.Contains(err.Error(), "还没到签到时间") {
		t.Errorf("提示文案不对：%v", err)
	}

	// 22:00 —— 在窗口内
	during := time.Date(2026, 11, 3, 22, 0, 0, 0, time.Local)
	if err := checkTaskWindowByClock(task, during); err != nil {
		t.Errorf("22:00 应可执行，得到：%v", err)
	}

	// 23:30 —— 已过期
	after := time.Date(2026, 11, 3, 23, 30, 0, 0, time.Local)
	if err := checkTaskWindowByClock(task, after); err == nil {
		t.Error("23:30 应判定为已过期")
	}
}

// TestParseTaskWindow 校验日期与时段的解析（含带星期后缀、跨天时段）。
func TestParseTaskWindow(t *testing.T) {
	// 带星期后缀
	w := parseTaskWindow(&store.Task{
		SignDate: "2026-11-03 (周二)", BeginClock: "21:00", EndClock: "23:00",
	})
	if !w.ok {
		t.Fatal("应能解析带星期后缀的日期")
	}
	if w.start.Hour() != 21 || w.end.Hour() != 23 {
		t.Errorf("时段解析错误：%v - %v", w.start, w.end)
	}

	// 跨天时段：结束时间应推到次日
	w2 := parseTaskWindow(&store.Task{
		SignDate: "2026-11-03", BeginClock: "23:30", EndClock: "00:30",
	})
	if !w2.ok {
		t.Fatal("应能解析跨天时段")
	}
	if w2.end.Day() != 4 {
		t.Errorf("跨天时段结束日期应为次日，得到 %v", w2.end)
	}

	// 缺字段 → 无法解析
	if parseTaskWindow(&store.Task{SignDate: "2026-11-03"}).ok {
		t.Error("缺少时段字段时不应解析成功")
	}
}

// TestHasRunnableTask 校验调度器的前置判断能正确排除未到时间的任务。
//
// 回归：计划刚创建时任务通常还没到签到时间，若不加前置判断就会
// 立刻发起一次必然失败的提交，并占用当天的执行机会。
func TestHasRunnableTask(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	// 没有任务 → 不可执行
	if svc.HasRunnableTask(ctx, acc.ID, "attendance") {
		t.Error("无任务时应返回 false")
	}

	now := time.Now()
	today := now.Format("2006-01-02")

	// 构造三种任务：未到时间 / 窗口内 / 已过期
	mk := func(name, begin, end string) *store.Task {
		return &store.Task{
			AccountID: acc.ID, Kind: "attendance", TaskName: name,
			SignInstanceWID: name, SignDate: today,
			BeginClock: begin, EndClock: end, SignStatus: "",
		}
	}
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{
		*mk("未到时间", "23:59", "23:59"),
	}); err != nil {
		t.Fatal(err)
	}
	if svc.HasRunnableTask(ctx, acc.ID, "attendance") {
		t.Error("只有未到时间的任务时应返回 false")
	}

	// 改成当前小时内的窗口 → 可执行
	cur := now.Format("15:04")
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{
		*mk("窗口内", cur, cur),
	}); err != nil {
		t.Fatal(err)
	}
	if !svc.HasRunnableTask(ctx, acc.ID, "attendance") {
		t.Error("窗口内的任务应返回 true")
	}

	// 已完成的任务不算可执行
	done := mk("已完成", cur, cur)
	done.Status = "done"
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{*done}); err != nil {
		t.Fatal(err)
	}
	if svc.HasRunnableTask(ctx, acc.ID, "attendance") {
		t.Error("已完成的任务不应算作可执行")
	}
}

// TestNextTaskWindow 校验从本地缓存算出任务窗口（自动计划的核心依据）。
//
// 场景来自实际使用：任务 21:00-23:00，用户 16:20 创建计划。
// 正确行为是算出「21:00 唤醒」，而不是立刻提交或反复同步接口试探。
func TestNextTaskWindow(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	// 无任务 → 无窗口
	if _, ok := svc.NextTaskWindow(ctx, acc.ID, "attendance"); ok {
		t.Error("无任务时不应返回窗口")
	}

	// 窗口起点取「当前时间 +1 小时」，任何时刻运行都正确
	// （原来写死 21:00-23:00，深夜运行时窗口已过导致测试失败）
	start := time.Now().Add(time.Hour)
	task := store.Task{
		AccountID: acc.ID, Kind: "attendance", TaskName: "2024级学生查寝打卡",
		SignInstanceWID: "w1",
		SignDate:        start.Format("2006-01-02"),
		BeginClock:      start.Format("15:04"),
		EndClock:        start.Add(2 * time.Hour).Format("15:04"),
	}
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{task}); err != nil {
		t.Fatal(err)
	}

	w, ok := svc.NextTaskWindow(ctx, acc.ID, "attendance")
	if !ok {
		t.Fatal("应返回任务窗口")
	}
	if !w.Start.Equal(start.Truncate(time.Minute)) {
		t.Errorf("窗口开始时间应为 %s，得到 %v", start.Format("15:04"), w.Start)
	}
	// Ready 取决于当前实际时间：只校验与当前时刻的一致性
	now := time.Now()
	expectReady := !now.Before(w.Start) && !now.After(w.End)
	if w.Ready != expectReady {
		t.Errorf("Ready 判定与当前时间不一致：now=%v ready=%v", now.Format("15:04"), w.Ready)
	}

	// 已签到（signStatus=4）不应算作可用窗口
	doneTask := task
	doneTask.SignInstanceWID = "w2"
	doneTask.TaskName = "已签到"
	doneTask.SignStatus = "4"
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{doneTask}); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.NextTaskWindow(ctx, acc.ID, "attendance"); ok {
		t.Error("已签到的任务不应返回窗口")
	}
}

// TestDescribeAttendanceWindow 校验探测提示描述的是查寝窗口，而不是笼统的任务数。
//
// 回归：提示曾显示「同步到 5 个任务」，但那 5 个里多数是签到/信息收集，
// 本项目并不处理；用户会误以为有 5 个查寝待打卡。
func TestDescribeAttendanceWindow(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	// windowTask 构造一个以 start/end 为窗口的任务。
	// SignDate 取 start 所在日期，跨天时段由 parseTaskWindow 处理，
	// 保证任何时刻运行测试都不会因窗口已过而误判。
	windowTask := func(wid, name string, start, end time.Time) store.Task {
		return store.Task{
			AccountID: acc.ID, Kind: "attendance", TaskName: name,
			SignInstanceWID: wid,
			SignDate:        start.Format("2006-01-02"),
			BeginClock:      start.Format("15:04"),
			EndClock:        end.Format("15:04"),
		}
	}

	// 无任务
	if got := describeAttendanceWindow(svc, ctx, acc.ID); !strings.Contains(got, "暂无") {
		t.Errorf("无任务时应提示暂无查寝，得到 %q", got)
	}

	// 有非查寝任务：不应被算作查寝
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{{
		AccountID: acc.ID, Kind: "sign", TaskName: "每日签到",
		SignInstanceWID: "s1",
		SignDate:        time.Now().Format("2006-01-02"),
		BeginClock:      "08:00", EndClock: "09:00",
	}}); err != nil {
		t.Fatal(err)
	}
	if got := describeAttendanceWindow(svc, ctx, acc.ID); !strings.Contains(got, "暂无") {
		t.Errorf("只有签到任务时应提示暂无查寝，得到 %q", got)
	}

	// 查寝任务在窗口内 → 提示可执行
	cur := time.Now()
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{
		windowTask("a1", "查寝", cur.Add(-time.Hour), cur.Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	if got := describeAttendanceWindow(svc, ctx, acc.ID); !strings.Contains(got, "可执行") {
		t.Errorf("窗口内应提示可执行，得到 %q", got)
	}

	// 未到窗口 → 提示开始时间
	start := cur.Add(4 * time.Hour)
	if err := svc.DB().ReplaceTasks(ctx, acc.ID, []store.Task{
		windowTask("a2", "查寝", start, start.Add(time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}
	if got := describeAttendanceWindow(svc, ctx, acc.ID); !strings.Contains(got, "开始") {
		t.Errorf("未到窗口应提示开始时间，得到 %q", got)
	}
}
