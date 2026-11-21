// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

// 验证码频控闸门。
//
// 背景（2026-11-18 实测）：学校的验证码接口有频控，重复请求返回
// code=20000015「验证过于频繁，请稍后再试！」。当天某账号在 7 分钟内
// 跑了 23 轮九宫格，其中 8 次就是被频控拒绝——每被拒一次就立刻再试，
// 限流窗口不断重置，实际成功率被拖到 42%。
//
// 单轮内部的间隔（16s）不足以解决这个问题，因为真正的放大源在轮与轮
// 之间：调度器重试、用户连点、待核对回填后的再次执行，都会各自开一轮。
// 所以在账号维度上设一道闸：同一时刻只跑一条链路，且被频控后退避一段时间。

import (
	"log"
	"sync"
	"time"
)

// captchaBackoff 是被频控后的退避时长。
//
// 学校声明的最小刷新间隔是 15 秒，但实测连撞会叠加惩罚，所以给到 90 秒：
// 足以让限流窗口彻底过去，又不至于让一个当晚窗口（通常 2 小时）错过太多。
var captchaBackoff = 90 * time.Second

// acquireCaptchaSlot 尝试占用该账号的验证码通道。
//
// 返回 ok=false 表示本次不应发起验证码流程：要么已有链路在跑，
// 要么正处于频控退避期。release 必须在占用成功后调用。
func (s *Service) acquireCaptchaSlot(accountID int64) (release func(), ok bool) {
	s.captchaGateMu.Lock()
	defer s.captchaGateMu.Unlock()
	if s.captchaGate == nil {
		s.captchaGate = map[int64]*captchaAccountGate{}
	}
	g := s.captchaGate[accountID]
	if g == nil {
		g = &captchaAccountGate{}
		s.captchaGate[accountID] = g
	}
	if g.busy {
		return nil, false
	}
	if !g.blockedUntil.IsZero() && time.Now().Before(g.blockedUntil) {
		return nil, false
	}
	g.busy = true
	return func() {
		s.captchaGateMu.Lock()
		g.busy = false
		s.captchaGateMu.Unlock()
	}, true
}

// noteCaptchaThrottled 记录该账号被验证码频控，接下来一段时间不再尝试。
func (s *Service) noteCaptchaThrottled(accountID int64) {
	s.captchaGateMu.Lock()
	defer s.captchaGateMu.Unlock()
	if s.captchaGate == nil {
		s.captchaGate = map[int64]*captchaAccountGate{}
	}
	g := s.captchaGate[accountID]
	if g == nil {
		g = &captchaAccountGate{}
		s.captchaGate[accountID] = g
	}
	g.blockedUntil = time.Now().Add(captchaBackoff)
	log.Printf("[captcha] 账号 %d 被验证码频控，%s 内不再尝试", accountID, captchaBackoff)
}

// clearCaptchaThrottle 在成功拿到 ticket 后清除退避状态。
func (s *Service) clearCaptchaThrottle(accountID int64) {
	s.captchaGateMu.Lock()
	defer s.captchaGateMu.Unlock()
	if g := s.captchaGate[accountID]; g != nil {
		g.blockedUntil = time.Time{}
	}
}

// captchaBlockedRemaining 返回该账号还要等多久才允许再试（0 表示不限制）。
func (s *Service) captchaBlockedRemaining(accountID int64) time.Duration {
	s.captchaGateMu.Lock()
	defer s.captchaGateMu.Unlock()
	g := s.captchaGate[accountID]
	if g == nil || g.blockedUntil.IsZero() {
		return 0
	}
	if d := time.Until(g.blockedUntil); d > 0 {
		return d
	}
	return 0
}

// tenantExpiredCooldown 是学校租户到期后的探测冷却时长。
//
// 到期是学校侧的合同状态，不会在几小时内恢复；每小时探一次只是白刷日志
// （实测 2026-11-19 武汉轻工一天 16 次无效请求）。给 6 小时后自然重试，
// 万一学校当天续订了也能自愈。
var tenantExpiredCooldown = 6 * time.Hour

// tenantExpiredAt 记录各账号被判为「租户到期」的时刻。
var tenantExpiredMu sync.Mutex
var tenantExpiredAt = map[int64]time.Time{}

// noteTenantExpired 记录该账号的租户已到期，冷却期内不再探测。
func (s *Service) noteTenantExpired(accountID int64) {
	tenantExpiredMu.Lock()
	defer tenantExpiredMu.Unlock()
	if _, ok := tenantExpiredAt[accountID]; ok {
		return // 已在冷却期，不重复记
	}
	tenantExpiredAt[accountID] = time.Now()
	log.Printf("[probe] 账号 %d 学校租户产品已到期，%s 内不再探测", accountID, tenantExpiredCooldown)
}

// tenantExpiredCooldownRemaining 返回冷却剩余时长（0 表示可探测）。
func (s *Service) tenantExpiredCooldownRemaining(accountID int64) time.Duration {
	tenantExpiredMu.Lock()
	defer tenantExpiredMu.Unlock()
	at, ok := tenantExpiredAt[accountID]
	if !ok {
		return 0
	}
	remain := tenantExpiredCooldown - time.Since(at)
	if remain <= 0 {
		// 冷却结束：清掉记录，允许重新探测（学校可能已续订）
		delete(tenantExpiredAt, accountID)
		return 0
	}
	return remain
}

// clearTenantExpired 在探测成功后清除到期标记。
func (s *Service) clearTenantExpired(accountID int64) {
	tenantExpiredMu.Lock()
	defer tenantExpiredMu.Unlock()
	delete(tenantExpiredAt, accountID)
}
