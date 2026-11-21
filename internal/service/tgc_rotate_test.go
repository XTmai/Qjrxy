// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

// 主动轮换与 App 会话冷却的单元测试。
//
// 背景见 tgc_rotate.go 头注释：三峡的票四天有效（长寿），燕山等 6 个
// 账号的票与 sessionToken 一起被作废（撞库四天）。这里锁定两类行为：
//   - 存龄超过阈值才轮换，未知存龄/新鲜票/冷却中一律不动；
//   - 判死后进入冷却，期间跳过，恢复（重登）后自动解除。

import (
	"errors"
	"testing"
	"time"

	"qjrxy/internal/store"
)

func TestTGCAge(t *testing.T) {
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, time.Local)
	cases := []struct {
		name    string
		account store.Account
		wantOK  bool
		wantAge time.Duration
	}{
		{
			name:    "正常存龄 13h",
			account: store.Account{TGCEnc: "x", TGCRefreshedAt: now.Add(-13 * time.Hour).Format(time.RFC3339)},
			wantOK:  true, wantAge: 13 * time.Hour,
		},
		{
			name:    "新票 1h",
			account: store.Account{TGCEnc: "x", TGCRefreshedAt: now.Add(-time.Hour).Format(time.RFC3339)},
			wantOK:  true, wantAge: time.Hour,
		},
		{name: "无票", account: store.Account{}, wantOK: false},
		{name: "有票无时间(老库)", account: store.Account{TGCEnc: "x"}, wantOK: false},
		{name: "时间格式坏", account: store.Account{TGCEnc: "x", TGCRefreshedAt: "not-a-time"}, wantOK: false},
	}
	for _, c := range cases {
		age, ok := tgcAge(&c.account, now)
		if ok != c.wantOK {
			t.Errorf("%s：ok=%v 期望 %v", c.name, ok, c.wantOK)
			continue
		}
		if ok && (age < c.wantAge-time.Minute || age > c.wantAge+time.Minute) {
			t.Errorf("%s：age=%v 期望约 %v", c.name, age, c.wantAge)
		}
	}
}

func TestAppSessionDeadCooldown(t *testing.T) {
	s := &Service{}
	aid := int64(9002)

	if d := s.appSessionDeadRemaining(aid); d != 0 {
		t.Fatalf("未判死时不应有冷却，实际 %v", d)
	}
	s.noteAppSessionDead(aid)
	if d := s.appSessionDeadRemaining(aid); d <= 0 {
		t.Fatalf("判死后应有冷却剩余，实际 %v", d)
	}
	// 重复标记不延长（每轮探测都撞 1202 时，不能把冷却无限续期）
	before := s.appSessionDeadRemaining(aid)
	s.noteAppSessionDead(aid)
	if after := s.appSessionDeadRemaining(aid); after > before+time.Second {
		t.Errorf("重复标记不应延长冷却：前 %v 后 %v", before, after)
	}
	// 解除
	s.clearAppSessionDead(aid)
	if d := s.appSessionDeadRemaining(aid); d != 0 {
		t.Errorf("解除后不应有冷却，实际 %v", d)
	}
}

func TestIsAppSessionDeadErr(t *testing.T) {
	if !isAppSessionDeadErr(errors.New("刷新 CAS 票被拒绝：当前未登录或session失效")) {
		t.Error("1202 文案应被识别")
	}
	for _, msg := range []string{"网络超时", "验证码错误", "刷新 CAS 票失败"} {
		if isAppSessionDeadErr(errors.New(msg)) {
			t.Errorf("%q 不应被识别为会话失效", msg)
		}
	}
	if isAppSessionDeadErr(nil) {
		t.Error("nil 不应被识别")
	}
}

func TestRotateTGCIfStaleSkipsFresh(t *testing.T) {
	// 新鲜票：不应轮换（也不会触碰网络）
	s := &Service{}
	a := &store.Account{
		ID: 9003, TGCEnc: "enc",
		TGCRefreshedAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
	}
	rotated, err := s.rotateTGCIfStale(nil, a)
	if err != nil {
		t.Fatalf("新鲜票不应报错：%v", err)
	}
	if rotated {
		t.Error("存龄 1h 的票不应触发轮换")
	}
	// 未知存龄（老库无 tgc_refreshed_at）：也不轮换
	old := &store.Account{ID: 9004, TGCEnc: "enc"}
	if rotated, _ := s.rotateTGCIfStale(nil, old); rotated {
		t.Error("未知存龄的票不应轮换（避免多余请求）")
	}
	// 无票：跳过
	noTGC := &store.Account{ID: 9005}
	if rotated, _ := s.rotateTGCIfStale(nil, noTGC); rotated {
		t.Error("无票账号不应轮换")
	}
}

func TestRotateTGCIfStaleRespectsCooldown(t *testing.T) {
	// 冷却中：即使存龄超阈值也不发请求（返回未轮换而非报错）
	s := &Service{}
	aid := int64(9006)
	s.noteAppSessionDead(aid)
	a := &store.Account{
		ID: aid, TGCEnc: "enc",
		TGCRefreshedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
		Cookies:        `{"sessionToken":"dead"}`,
	}
	rotated, err := s.rotateTGCIfStale(nil, a)
	if err != nil {
		t.Fatalf("冷却中应静默跳过而非报错：%v", err)
	}
	if rotated {
		t.Error("冷却中不应轮换")
	}
}

func TestAppSessionDeadSkipResult(t *testing.T) {
	a := &store.Account{ID: 9007, Alias: "测试", LoginStatus: "expired"}
	res := appSessionDeadSkipResult(a, 5*time.Hour)
	if !res.Skipped {
		t.Error("应为 Skipped，避免计入「需处理」")
	}
	if res.Status != "expired" {
		t.Errorf("应保持原状态，实际 %q", res.Status)
	}
	if want := "重新短信登录"; !containsStr(res.Message, want) {
		t.Errorf("提示应指引重新短信登录，实际 %q", res.Message)
	}
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && indexOfStr(s, sub) >= 0
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
