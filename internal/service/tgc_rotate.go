// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

// CAS 票（TGC）的主动轮换与 sessionToken 失效冷却。
//
// 背景（2026-11-19~21 生产日志复盘）：
//
//   - 三峡大学科技学院（CLOUD/短信）的账号能一直续期：它的 TGC 四天
//     没换过、学校一直认——寿命由学校 CAS 配置决定，且「持续使用」
//     会刷新活跃度。
//   - 燕山等 6 个 NOTCLOUD 账号则整批失效：TGC 与 sessionToken 同时
//     被作废，程序每小时拿同一张死票去撞（一天 7~8 轮无效请求）。
//
// 2026-11-19 的只读探测实验证明了出路：用活着的 sessionToken 调
// validation/v-9012，服务端一次性签发了一张**全新的 TGC**
// （TGT-4152-…，库里存的还是旧的 TGT-55…）。也就是说：
//
//	只要 sessionToken 活着，TGC 层面可以随时「重新出厂」，
//	不需要用户再收一次短信。
//
// 因此做两件事：
//
//  1. 主动轮换：探测时发现 TGC 存龄超过阈值，就换一张新的落库。
//     学校侧永远看到活跃票，避免「长期闲置 → 被回收 → 整批断」。
//  2. 失效冷却：v-9012 返回 1202（session 失效）说明根凭据已死，
//     重试永远无效。记入冷却，期间探测直接跳过，账号卡片显示
//     「需重新短信登录」，不再每小时撞学校登录页。

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/store"
)

// tgcRotateThreshold 是 CAS 票的主动轮换阈值。
//
// 学校 TGC 的实际寿命从数小时到数周不等（三峡实测 ≥4 天）。阈值取 12h：
// 远小于观察到的最长寿命，又能保证「活跃账号的票永远不超过半天龄」；
// 探测循环本身每小时跑一次，轮换实际发生在存龄刚过 12h 的那一轮。
var tgcRotateThreshold = 12 * time.Hour

// appSessionDeadCooldown 是 sessionToken 判死后的冷却时长。
//
// v-9012 返回 1202 意味着根凭据（App 账号会话）已被作废——学生退出
// 登录、换设备都会触发，服务端无法自愈。冷却 6h：不撞库，又能在
// 用户重新登录后较短时间内自动恢复探测。
var appSessionDeadCooldown = 6 * time.Hour

// appSessionDeadAt 记录各账号 sessionToken 被判死的时刻。
var appSessionMu sync.Mutex
var appSessionDeadAt = map[int64]time.Time{}

// noteAppSessionDead 记录该账号的 App 会话已失效，冷却期内跳过自动恢复。
func (s *Service) noteAppSessionDead(accountID int64) {
	appSessionMu.Lock()
	defer appSessionMu.Unlock()
	if _, ok := appSessionDeadAt[accountID]; ok {
		return // 已在冷却期
	}
	appSessionDeadAt[accountID] = time.Now()
	log.Printf("[auth] 账号 %d 的 App 会话(sessionToken)已失效，%s 内不再自动尝试",
		accountID, appSessionDeadCooldown)
}

// appSessionDeadRemaining 返回 App 会话冷却剩余时长（0 表示可尝试）。
func (s *Service) appSessionDeadRemaining(accountID int64) time.Duration {
	appSessionMu.Lock()
	defer appSessionMu.Unlock()
	at, ok := appSessionDeadAt[accountID]
	if !ok {
		return 0
	}
	if remain := appSessionDeadCooldown - time.Since(at); remain > 0 {
		return remain
	}
	delete(appSessionDeadAt, accountID)
	return 0
}

// clearAppSessionDead 在会话恢复（重登/续期成功）后解除冷却。
func (s *Service) clearAppSessionDead(accountID int64) {
	appSessionMu.Lock()
	defer appSessionMu.Unlock()
	delete(appSessionDeadAt, accountID)
}

// isAppSessionDeadErr 判断错误是否为「App 会话已失效」（v-9012 的 1202）。
func isAppSessionDeadErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "当前未登录或session失效")
}

// tgcAge 计算 CAS 票的存龄。空/无法解析的时刻视为「未知存龄」，
// 返回 ok=false——未知时不主动轮换，避免对刚登录的票多余折腾。
func tgcAge(a *store.Account, now time.Time) (age time.Duration, ok bool) {
	at := strings.TrimSpace(a.TGCRefreshedAt)
	if at == "" || strings.TrimSpace(a.TGCEnc) == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return 0, false
	}
	return now.Sub(t), true
}

// rotateTGCIfStale 在探测路径上按存龄主动轮换 CAS 票。
//
// 前提：账号有 sessionToken（轮换的根凭据）。没有它（纯密码账号），
// 轮换无从谈起，直接跳过——这类账号的恢复本来就靠密码重登。
// 轮换成功会顺带把新业务会话落库（renewSessionWithTGC 内完成），
// 因此失败时的返回错误由调用方决定如何呈现。
func (s *Service) rotateTGCIfStale(ctx context.Context, account *store.Account) (rotated bool, err error) {
	if strings.TrimSpace(account.TGCEnc) == "" {
		return false, nil // 没有票可轮换（走 v-9012 首签，不在此处）
	}
	age, ok := tgcAge(account, time.Now())
	if !ok || age < tgcRotateThreshold {
		return false, nil
	}
	if s.appSessionDeadRemaining(account.ID) > 0 {
		return false, nil // 根凭据已死，轮换必然失败
	}
	log.Printf("[auth] 账号 %d 的 CAS 票存龄 %s（超过 %s），主动轮换",
		account.ID, age.Round(time.Minute), tgcRotateThreshold)
	if err := s.renewSessionWithTGC(ctx, account); err != nil {
		if isAppSessionDeadErr(err) {
			s.noteAppSessionDead(account.ID)
			return false, errors.New("App 会话已失效，请重新发送短信验证码登录")
		}
		return false, err
	}
	// 轮换成功即证明根凭据活着，解除可能存在的冷却。
	s.clearAppSessionDead(account.ID)
	return true, nil
}

// appSessionDeadSkipResult 生成「冷却期内跳过」的探测结果。
func appSessionDeadSkipResult(account *store.Account, remain time.Duration) ProbeResult {
	return ProbeResult{
		AccountID:   account.ID,
		AccountName: account.Alias,
		Status:      account.LoginStatus,
		Message:     "App 会话已失效，需重新短信登录（自动尝试已暂停 " + remain.Round(time.Minute).String() + "）",
		Skipped:     true,
	}
}

// ensure cpdaily import is used for doc reference.
var _ = cpdaily.FlowSMSRefreshTGC
