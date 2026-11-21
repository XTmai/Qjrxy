// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

// 验证码频控闸门的单元测试。
//
// 背景：2026-11-18 生产实测，同一账号 7 分钟内跑了 23 轮验证码，
// 其中 8 次被学校频控拒绝（20000015「验证过于频繁」）。根因是轮与轮
// 之间没有任何协调：调度重试、用户连点、待核对回填都会各自开一轮。
// 闸门负责两件事：同账号串行、被频控后退避。

import (
	"testing"
	"time"
)

// TestCaptchaGateSerializesSameAccount 校验同一账号同时只能跑一条链路。
func TestCaptchaGateSerializesSameAccount(t *testing.T) {
	s := &Service{}
	release1, ok1 := s.acquireCaptchaSlot(1)
	if !ok1 {
		t.Fatal("首次占用应成功")
	}
	// 第二次（并发触发）必须被拒，否则两路会互相加重频控
	if _, ok2 := s.acquireCaptchaSlot(1); ok2 {
		t.Error("同一账号已有链路在跑时，第二次占用应失败")
	}
	// 不同账号互不影响
	if _, ok3 := s.acquireCaptchaSlot(2); !ok3 {
		t.Error("不同账号不应互相阻塞")
	}
	release1()
	// 释放后可以再次占用
	release2, ok4 := s.acquireCaptchaSlot(1)
	if !ok4 {
		t.Error("释放后应可再次占用")
	}
	release2()
}

// TestCaptchaGateBackoff 校验被频控后一段时间内直接拒绝。
func TestCaptchaGateBackoff(t *testing.T) {
	s := &Service{}
	s.noteCaptchaThrottled(7)

	if d := s.captchaBlockedRemaining(7); d <= 0 {
		t.Fatalf("频控后应记录剩余退避时间，实际 %v", d)
	}
	if _, ok := s.acquireCaptchaSlot(7); ok {
		t.Error("退避期内不应允许再开验证码链路")
	}
	// 其它账号不受影响
	rel, ok := s.acquireCaptchaSlot(8)
	if !ok {
		t.Error("退避只应作用于被频控的账号")
	}
	rel()

	// 成功一次后应清除退避
	s.clearCaptchaThrottle(7)
	if d := s.captchaBlockedRemaining(7); d != 0 {
		t.Errorf("清除后不应再有退避，实际 %v", d)
	}
	if rel2, ok := s.acquireCaptchaSlot(7); !ok {
		t.Error("清除退避后应可再次占用")
	} else {
		rel2()
	}
}

// TestCaptchaGateBackoffExpires 校验退避到期后自动恢复。
func TestCaptchaGateBackoffExpires(t *testing.T) {
	s := &Service{}
	// 直接构造一个已过期的解禁时刻，避免测试真的等 90 秒
	s.captchaGateMu.Lock()
	s.captchaGate = map[int64]*captchaAccountGate{
		9: {blockedUntil: time.Now().Add(-time.Second)},
	}
	s.captchaGateMu.Unlock()

	if d := s.captchaBlockedRemaining(9); d != 0 {
		t.Errorf("已过期的退避不应再报告剩余时间，实际 %v", d)
	}
	rel, ok := s.acquireCaptchaSlot(9)
	if !ok {
		t.Error("退避到期后应允许占用")
	}
	rel()
}

// TestCaptchaGateMissingAccount 校验未记录的账号不受限制。
func TestCaptchaGateMissingAccount(t *testing.T) {
	s := &Service{}
	if d := s.captchaBlockedRemaining(42); d != 0 {
		t.Errorf("从未频控的账号不应有退避，实际 %v", d)
	}
	rel, ok := s.acquireCaptchaSlot(42)
	if !ok {
		t.Error("首次使用应允许")
	}
	rel()
}

// TestTaskModeAndTenantExpiredClassifiers 校验两个确定性终止码的判定。
func TestTaskModeAndTenantExpiredClassifiers(t *testing.T) {
	if !IsTaskModeUnsupported("sign_mode_nfc") {
		t.Error("NFC 任务应被识别为不支持自动化")
	}
	if !IsTaskModeUnsupported("sign_mode_qrcode") {
		t.Error("二维码任务应被识别为不支持自动化")
	}
	for _, code := range []string{"0", "", "2210060000", "captcha_throttled"} {
		if IsTaskModeUnsupported(code) {
			t.Errorf("code=%q 不应被识别为任务类型不支持", code)
		}
	}
	if !IsTenantExpired("tenant_expired") {
		t.Error("tenant_expired 应被识别")
	}
	if IsTenantExpired("0") || IsTenantExpired("") {
		t.Error("普通 code 不应被识别为租户到期")
	}
}

// TestTenantExpiredCooldown 校验租户到期后探测被跳过，且冷却到期后自动恢复。
//
// 回归（2026-11-19 生产）：420「产品已到期」的账号仍在每小时被探测，
// 一天 16 次无效请求。到期是学校侧合同状态，几小时内不会变，
// 探测纯属白刷日志。
func TestTenantExpiredCooldown(t *testing.T) {
	// 每个用例独立，避免与其它用例共享包级 map
	accountID := int64(9001)
	s := &Service{}

	if d := s.tenantExpiredCooldownRemaining(accountID); d != 0 {
		t.Fatalf("未标记到期时不应有冷却，实际 %v", d)
	}

	s.noteTenantExpired(accountID)
	if d := s.tenantExpiredCooldownRemaining(accountID); d <= 0 {
		t.Fatalf("标记后应有冷却剩余，实际 %v", d)
	}

	// 冷却期内重复标记不应延长（避免每次探测都重置窗口）
	before := s.tenantExpiredCooldownRemaining(accountID)
	s.noteTenantExpired(accountID)
	after := s.tenantExpiredCooldownRemaining(accountID)
	if after > before+time.Second {
		t.Errorf("重复标记不应延长冷却：前 %v 后 %v", before, after)
	}

	// 清除后应立即可探测
	s.clearTenantExpired(accountID)
	if d := s.tenantExpiredCooldownRemaining(accountID); d != 0 {
		t.Errorf("清除后不应有冷却，实际 %v", d)
	}
}
