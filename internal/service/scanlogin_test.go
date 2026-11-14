package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"qjrxy/internal/store"
)

// resetScanLoginTickets 清空进程内票据表，避免用例之间互相污染。
func resetScanLoginTickets() {
	scanLoginMu.Lock()
	scanLoginTickets = map[string]ScanLoginTicket{}
	scanLoginMu.Unlock()
}

// TestScanLoginTicketLifecycle 校验「建票据 → 轮询 pending → 提交 → 轮询命中」。
func TestScanLoginTicketLifecycle(t *testing.T) {
	resetScanLoginTickets()
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	ticket, _, err := svc.CreateScanLoginTicket(ctx, user.ID, acc.ID)
	if err != nil {
		t.Fatalf("建票据失败：%v", err)
	}

	// 扫码前：pending
	pending, res, err := svc.PollScanLogin(ticket)
	if err != nil {
		t.Fatalf("轮询不应报错：%v", err)
	}
	if !pending || res != nil {
		t.Error("未扫码时应返回 pending=true 且结果为 nil")
	}

	// 手机页要能读到目标账号信息，用于二次确认
	school, username, _, err := svc.ScanLoginTicketTarget(ctx, ticket)
	if err != nil {
		t.Fatalf("读取目标账号失败：%v", err)
	}
	if school != acc.SchoolName || username != acc.Username {
		t.Errorf("目标账号不符：got %s/%s", school, username)
	}

	// 提交
	err = svc.SubmitScanLogin(ticket, ScanLoginResult{
		Ticket:    "ST-iap:test-123456",
		AppUserID: "app-user-1",
		Host:      "t.campusphere.net",
		Device:    DeviceInfo{DeviceID: "DEVICE-IDENTIFIER-01", SystemName: "ios"},
	})
	if err != nil {
		t.Fatalf("提交失败：%v", err)
	}

	pending, res, err = svc.PollScanLogin(ticket)
	if err != nil {
		t.Fatalf("轮询不应报错：%v", err)
	}
	if pending || res == nil {
		t.Fatal("已提交后应返回 pending=false 且带结果")
	}
	if res.Ticket != "ST-iap:test-123456" {
		t.Errorf("票据载荷丢失：%q", res.Ticket)
	}
	if res.Device.Platform != "ios" {
		t.Errorf("平台归一化错误：%q", res.Device.Platform)
	}
}

// TestScanLoginTicketBelongsTo 校验票据的账号归属，防止多个弹窗串号。
func TestScanLoginTicketBelongsTo(t *testing.T) {
	resetScanLoginTickets()
	svc, user := newTestService(t)
	accA := newTestAccount(t, svc, user.ID)
	accB, err := svc.DB().CreateAccount(context.Background(), &store.Account{
		UserID: user.ID, SchoolName: "另一所大学", Username: "2024002", Host: "b.campusphere.net",
	})
	if err != nil {
		t.Fatalf("建第二个账号失败：%v", err)
	}
	ctx := context.Background()

	ticket, _, err := svc.CreateScanLoginTicket(ctx, user.ID, accA.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ScanLoginTicketBelongsTo(ctx, user.ID, accA.ID, ticket); err != nil {
		t.Errorf("同账号应通过：%v", err)
	}
	if err := svc.ScanLoginTicketBelongsTo(ctx, user.ID, accB.ID, ticket); err == nil {
		t.Error("跨账号应被拒绝，否则 A 的票据会被写进 B")
	}
	if err := svc.ScanLoginTicketBelongsTo(ctx, user.ID+999, accA.ID, ticket); err == nil {
		t.Error("跨用户应被拒绝")
	}
}

// TestScanLoginInvalidTicket 校验无效票据的报错。
func TestScanLoginInvalidTicket(t *testing.T) {
	resetScanLoginTickets()
	svc, _ := newTestService(t)
	if _, _, err := svc.PollScanLogin("no-such-ticket"); err == nil {
		t.Error("无效票据应报错")
	}
	if _, _, _, err := svc.ScanLoginTicketTarget(context.Background(), "no-such-ticket"); err == nil {
		t.Error("无效票据读取目标应报错")
	}
	if err := svc.SubmitScanLogin("no-such-ticket", ScanLoginResult{Ticket: "x"}); err == nil {
		t.Error("无效票据提交应报错")
	}
}

// TestScanLoginExpiredTicket 校验过期票据被清理。
func TestScanLoginExpiredTicket(t *testing.T) {
	resetScanLoginTickets()
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)

	ticket, _, err := svc.CreateScanLoginTicket(context.Background(), user.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 手动把票据改成已过期
	scanLoginMu.Lock()
	tk := scanLoginTickets[ticket]
	tk.ExpiresAt = time.Now().Add(-time.Minute)
	scanLoginTickets[ticket] = tk
	scanLoginMu.Unlock()

	if _, _, err := svc.PollScanLogin(ticket); err == nil {
		t.Error("过期票据应报错")
	}
	// 过期票据应被顺手清掉，再查一次仍是「无效」而不是「过期」
	if _, _, err := svc.PollScanLogin(ticket); err == nil {
		t.Error("过期票据第二次查询应报错")
	}
}

// TestSubmitScanLoginValidation 校验提交时的字段校验。
func TestSubmitScanLoginValidation(t *testing.T) {
	resetScanLoginTickets()
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	ticket, _, err := svc.CreateScanLoginTicket(ctx, user.ID, acc.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 空票据
	if err := svc.SubmitScanLogin(ticket, ScanLoginResult{}); err == nil {
		t.Error("空登录票据应被拒绝")
	}
	// 超长
	long := strings.Repeat("A", 3000)
	if err := svc.SubmitScanLogin(ticket, ScanLoginResult{Ticket: long}); err == nil {
		t.Error("超长票据应被拒绝")
	}
	// 含空白
	if err := svc.SubmitScanLogin(ticket, ScanLoginResult{Ticket: "ST iap"}); err == nil {
		t.Error("含空白的票据应被拒绝")
	}
	// 设备 ID 过短
	err = svc.SubmitScanLogin(ticket, ScanLoginResult{
		Ticket: "ST-iap:ok", Device: DeviceInfo{DeviceID: "short"},
	})
	if err == nil {
		t.Error("过短的设备 ID 应被拒绝")
	}
}
