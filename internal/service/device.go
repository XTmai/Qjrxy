// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/events"
	"qjrxy/internal/secure"
	"qjrxy/internal/store"
)

// DeviceInfo 是扫码页采集到的完整设备信息。
type DeviceInfo struct {
	DeviceID      string `json:"deviceId"`
	UUID          string `json:"uuid"`
	DeviceModel   string `json:"deviceModel"`
	SystemVersion string `json:"systemVersion"`
	BuildID       string `json:"buildId"`
	SystemName    string `json:"systemName"`
	Platform      string `json:"platform"`
	AppVersion    string `json:"appVersion"`
	WisDeviceID   string `json:"wisDeviceId"`
	UserAgent     string `json:"userAgent"`
	CollectedAt   string `json:"collectedAt"`
}

// DeviceTicket 是一次「扫码获取设备信息」的授权票据。
//
// 采用「扫码页写入 + 主页面轮询读取」的模式：
// 电脑端生成票据并展示二维码，手机扫码后由 App 内 WebView 提交设备信息，
// 电脑端轮询到结果后自动回填，无需用户手动刷新或复制粘贴。
type DeviceTicket struct {
	UserID    int64
	AccountID int64
	ExpiresAt time.Time
	Result    *DeviceInfo
}

// deviceTickets 保存待回填的设备票据，进程内有效。
var (
	deviceMu      sync.Mutex
	deviceTickets = map[string]DeviceTicket{}
)

// ticketTTL 票据有效期。
const ticketTTL = 15 * time.Minute

// CreateDeviceTicket 生成设备扫码票据。
func (s *Service) CreateDeviceTicket(ctx context.Context, userID, accountID int64) (string, time.Time, error) {
	if _, err := s.db.AccountByID(ctx, userID, accountID); err != nil {
		return "", time.Time{}, err
	}
	ticket := secure.RandomID(24)
	expires := time.Now().Add(ticketTTL)

	deviceMu.Lock()
	// 顺带清理过期票据，避免内存堆积
	for id, t := range deviceTickets {
		if time.Now().After(t.ExpiresAt) {
			delete(deviceTickets, id)
		}
	}
	deviceTickets[ticket] = DeviceTicket{UserID: userID, AccountID: accountID, ExpiresAt: expires}
	deviceMu.Unlock()
	return ticket, expires, nil
}

// SubmitDeviceInfo 扫码页提交设备信息（无需登录，凭票据授权）。
//
// 这里不删除票据——主页面要靠轮询来读取结果。
func (s *Service) SubmitDeviceInfo(ticket string, info DeviceInfo) error {
	// 先验票据（这是授权凭据），再验字段：伪造票据不该拿到字段级的错误提示。
	deviceMu.Lock()
	defer deviceMu.Unlock()

	t, ok := deviceTickets[ticket]
	if !ok {
		return errors.New("扫码票据无效，请在电脑端重新生成二维码")
	}
	if time.Now().After(t.ExpiresAt) {
		delete(deviceTickets, ticket)
		return errors.New("二维码已过期，请在电脑端重新生成")
	}

	info.DeviceID = strings.TrimSpace(info.DeviceID)
	// 与 validateDeviceInput 同口径：扫码授权是生产上唯一真正写入
	// CpUUID 的路径，不能只查非空就把任意长度/带控制符的值放进提交请求。
	if len(info.DeviceID) < 8 || len(info.DeviceID) > 128 {
		return errors.New("设备 ID 长度需在 8-128 之间")
	}
	for _, r := range info.DeviceID {
		if r <= 0x20 || r == 0x7f {
			return errors.New("设备 ID 不能包含空白或控制字符")
		}
	}
	if len(info.UserAgent) > 512 {
		return errors.New("User-Agent 过长（最多 512 字符）")
	}

	info.CollectedAt = time.Now().Format(time.RFC3339)
	// 票据载荷里保留归一化结果供前端展示；落库是否覆盖平台由
	// CollectDeviceInfo 按原始 SystemName 显式判断（见该函数注释）。
	info.Platform = normalizePlatform(info.SystemName)
	t.Result = &info
	deviceTickets[ticket] = t
	return nil
}

// PollDeviceTicket 主页面轮询票据状态。
//
// pending=true 表示还没扫码；info 非空表示已采集到设备信息。
func (s *Service) PollDeviceTicket(ticket string) (pending bool, info *DeviceInfo, err error) {
	deviceMu.Lock()
	defer deviceMu.Unlock()

	t, ok := deviceTickets[ticket]
	if !ok {
		return false, nil, errors.New("票据无效或已失效，请重新生成二维码")
	}
	if time.Now().After(t.ExpiresAt) {
		delete(deviceTickets, ticket)
		return false, nil, errors.New("二维码已过期，请重新生成")
	}
	if t.Result == nil {
		return true, nil, nil
	}
	return false, t.Result, nil
}

// CollectDeviceInfo 把轮询到的设备信息落库，并消费该票据。
func (s *Service) CollectDeviceInfo(ctx context.Context, userID int64, ticket string) (*store.Account, error) {
	deviceMu.Lock()
	t, ok := deviceTickets[ticket]
	if !ok || t.Result == nil {
		deviceMu.Unlock()
		return nil, errors.New("没有可应用的设备信息，请先扫码")
	}
	if t.UserID != userID {
		deviceMu.Unlock()
		return nil, errors.New("票据不属于当前用户")
	}
	info := *t.Result
	delete(deviceTickets, ticket)
	deviceMu.Unlock()

	account, err := s.db.AccountByID(ctx, userID, t.AccountID)
	if err != nil {
		return nil, err
	}

	deviceID := strings.TrimSpace(info.DeviceID)
	// 机型/版本/构建号/指纹：只在扫码确实取到时覆盖，避免把已有的好值冲成空
	deviceModel := account.DeviceModel
	if v := strings.TrimSpace(info.DeviceModel); v != "" && v != "unknown" {
		deviceModel = v
	}
	systemVersion := account.SystemVersion
	if v := strings.TrimSpace(info.SystemVersion); v != "" && v != "unknown" {
		systemVersion = v
	}
	buildID := account.BuildID
	if v := strings.TrimSpace(info.BuildID); v != "" {
		buildID = v
	}
	wisDeviceID := account.WisDeviceID
	if v := strings.TrimSpace(info.WisDeviceID); v != "" {
		wisDeviceID = v
	}
	userAgent := account.UserAgent
	if v := strings.TrimSpace(info.UserAgent); v != "" {
		userAgent = v
	}
	appVersion := account.AppVersion
	if v := strings.TrimSpace(info.AppVersion); v != "" {
		// 之前漏掉这个字段：扫码页明明上报了 appVersion 却从未落库，
		// 导致提交请求里 UA 与 Cpdaily-Extension 的 appVersion 互相矛盾。
		appVersion = v
	}
	// 平台只在扫码确实上报了可识别的系统名时才覆盖：
	// normalizePlatform 对未知值一律返回 android，直接拿来覆盖会把 iOS 账号改成 android，
	// 之后所有提交静默切到 Android 协议。
	platform := account.Platform
	switch strings.ToLower(strings.TrimSpace(info.SystemName)) {
	case "ios", "iphone", "ipad":
		platform = "ios"
	case "android":
		platform = "android"
	}

	if err := s.db.UpdateAccountDevice(ctx, userID, account.ID, store.AccountDevice{
		DeviceID: deviceID, DeviceModel: deviceModel, SystemVersion: systemVersion,
		Platform: platform, WisDeviceID: wisDeviceID, UserAgent: userAgent,
		AppVersion: appVersion, BuildID: buildID,
	}); err != nil {
		return nil, err
	}
	// 扫码授权常在手机端完成：桌面端账号卡片应立刻从「未绑定设备」变成已绑定。
	s.notify(userID, events.TopicAccounts)
	return s.db.AccountByID(ctx, userID, account.ID)
}

// ChangePassword 修改平台登录密码。
//
// 改密后使该用户其他设备上的会话立即失效（保留当前这一次），
// 否则账号被盗后改密并不能把入侵者踢下线。
func (s *Service) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword, currentToken string) error {
	if len(newPassword) < 6 {
		return errors.New("新密码至少 6 位")
	}
	user, err := s.db.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !secure.VerifyPassword(oldPassword, user.PasswordHash) {
		return errors.New("原密码不正确")
	}
	hash, err := secure.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.db.UpdateUserPassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.db.DeleteOtherSessions(ctx, userID, currentToken)
}
