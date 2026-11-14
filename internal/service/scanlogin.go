package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/events"
	"qjrxy/internal/secure"
	"qjrxy/internal/store"
)

// ScanLoginResult 是手机扫码页换取到的登录态载荷。
//
// 票据由学校在 App 原生 cookie 容器里下发，服务端只负责接收与落库：
// 原来的做法是复刻学校登录表单（密码/滑块/页内短信），对 NOTCLOUD 学校
// 的自定义认证页无能为力；改成「借 App 的容器换票」后不需要任何登录因子。
type ScanLoginResult struct {
	// Ticket 是学校业务票据 MOD_AUTH_CAS，等同一次真实登录的产物。
	Ticket string `json:"ticket"`
	// AppUserID 是今日校园 App 账号体系的 userId，用来防止把别人的登录态
	// 绑到这条账号记录上（同一记录两次扫码 userId 不同即拒绝）。
	AppUserID string `json:"appUserId"`
	// Host 是学校门户域名，落库时补进账号（部分账号创建时 host 为空）。
	Host   string     `json:"host"`
	School string     `json:"school"`
	Device DeviceInfo `json:"device"`
}

// ScanLoginTicket 是一次「扫码获取登录态」的授权票据。
//
// 与 DeviceTicket 同为「手机写入 + 桌面轮询读取」，但两者独立存放：
// 一个负责设备指纹，一个负责登录态，互不影响。
type ScanLoginTicket struct {
	UserID    int64
	AccountID int64
	ExpiresAt time.Time
	Result    *ScanLoginResult
}

// scanLoginTickets 保存待回填的扫码登录票据，进程内有效。
var (
	scanLoginMu      sync.Mutex
	scanLoginTickets = map[string]ScanLoginTicket{}
)

// CreateScanLoginTicket 生成扫码登录票据。
func (s *Service) CreateScanLoginTicket(ctx context.Context, userID, accountID int64) (string, time.Time, error) {
	if _, err := s.db.AccountByID(ctx, userID, accountID); err != nil {
		return "", time.Time{}, err
	}
	ticket := secure.RandomID(24)
	expires := time.Now().Add(ticketTTL)

	scanLoginMu.Lock()
	// 顺带清理过期票据，避免内存堆积
	for id, t := range scanLoginTickets {
		if time.Now().After(t.ExpiresAt) {
			delete(scanLoginTickets, id)
		}
	}
	scanLoginTickets[ticket] = ScanLoginTicket{UserID: userID, AccountID: accountID, ExpiresAt: expires}
	scanLoginMu.Unlock()
	return ticket, expires, nil
}

// ScanLoginTicketTarget 返回票据对应的账号展示信息（供手机页确认）。
//
// 只给学校名/学号/备注：手机页要大字告诉用户「即将绑定到哪条记录」，
// 用户看清后才点确认。绝不能在这里带出 cookie 或密码。
func (s *Service) ScanLoginTicketTarget(ctx context.Context, ticket string) (schoolName, username, alias string, err error) {
	scanLoginMu.Lock()
	t, ok := scanLoginTickets[ticket]
	scanLoginMu.Unlock()
	if !ok {
		return "", "", "", errors.New("扫码票据无效，请在电脑端重新生成二维码")
	}
	if time.Now().After(t.ExpiresAt) {
		scanLoginMu.Lock()
		delete(scanLoginTickets, ticket)
		scanLoginMu.Unlock()
		return "", "", "", errors.New("二维码已过期，请在电脑端重新生成")
	}
	account, err := s.db.AccountByID(ctx, t.UserID, t.AccountID)
	if err != nil {
		return "", "", "", err
	}
	return account.SchoolName, account.Username, account.Alias, nil
}

// SubmitScanLogin 扫码页提交票据（无需登录，凭票据授权）。
//
// 这里不删除票据——主页面要靠轮询来读取结果，爽约的票据交给 TTL 清理。
func (s *Service) SubmitScanLogin(ticket string, result ScanLoginResult) error {
	result.Ticket = strings.TrimSpace(result.Ticket)
	result.AppUserID = strings.TrimSpace(result.AppUserID)
	result.Host = cpdaily.NormalizeHost(result.Host)

	scanLoginMu.Lock()
	defer scanLoginMu.Unlock()

	t, ok := scanLoginTickets[ticket]
	if !ok {
		return errors.New("扫码票据无效，请在电脑端重新生成二维码")
	}
	if time.Now().After(t.ExpiresAt) {
		delete(scanLoginTickets, ticket)
		return errors.New("二维码已过期，请在电脑端重新生成")
	}

	// 票据是这次登录的唯一凭据，先验它在不在，再看长度：
	// 伪造票据不该拿到字段级的错误提示。
	if result.Ticket == "" {
		return errors.New("没有拿到学校的登录票据，请确认手机上的今日校园已登录")
	}
	if len(result.Ticket) > 2048 {
		return errors.New("登录票据过长，疑似异常")
	}
	for _, r := range result.Ticket {
		if r <= 0x20 || r == 0x7f {
			return errors.New("登录票据格式异常")
		}
	}

	// 设备字段复用设备扫码的同一套校验口径。
	result.Device.DeviceID = strings.TrimSpace(result.Device.DeviceID)
	if result.Device.DeviceID != "" {
		if len(result.Device.DeviceID) < 8 || len(result.Device.DeviceID) > 128 {
			return errors.New("设备 ID 长度需在 8-128 之间")
		}
		for _, r := range result.Device.DeviceID {
			if r <= 0x20 || r == 0x7f {
				return errors.New("设备 ID 不能包含空白或控制字符")
			}
		}
	}
	if len(result.Device.UserAgent) > 512 {
		return errors.New("User-Agent 过长（最多 512 字符）")
	}
	result.Device.CollectedAt = time.Now().Format(time.RFC3339)
	result.Device.Platform = normalizePlatform(result.Device.SystemName)

	t.Result = &result
	scanLoginTickets[ticket] = t
	return nil
}

// ScanLoginTicketBelongsTo 校验票据确实属于该用户与账号。
//
// 前端可能同时开多个扫码弹窗，若不校验，A 账号的票据可能被 B 账号的轮询消费，
// 把登录态写进错的记录。
func (s *Service) ScanLoginTicketBelongsTo(_ context.Context, userID, accountID int64, ticket string) error {
	scanLoginMu.Lock()
	t, ok := scanLoginTickets[ticket]
	scanLoginMu.Unlock()
	if !ok {
		return errors.New("票据无效或已失效，请重新生成二维码")
	}
	if t.UserID != userID || t.AccountID != accountID {
		return errors.New("票据不属于当前账号，请重新生成二维码")
	}
	if time.Now().After(t.ExpiresAt) {
		return errors.New("二维码已过期，请重新生成")
	}
	return nil
}

// PollScanLogin 桌面端轮询扫码登录结果。
//
// pending=true 表示还没扫码；result 非空表示扫码页已提交。
func (s *Service) PollScanLogin(ticket string) (pending bool, result *ScanLoginResult, err error) {
	scanLoginMu.Lock()
	defer scanLoginMu.Unlock()

	t, ok := scanLoginTickets[ticket]
	if !ok {
		return false, nil, errors.New("票据无效或已失效，请重新生成二维码")
	}
	if time.Now().After(t.ExpiresAt) {
		delete(scanLoginTickets, ticket)
		return false, nil, errors.New("二维码已过期，请重新生成")
	}
	if t.Result == nil {
		return true, nil, nil
	}
	return false, t.Result, nil
}

// CollectScanLogin 把轮询到的登录态落库，并消费该票据。
//
// 落库分三件事：写学校票据（finishSession 负责验证 + 落 cookies）、
// 补 host（部分账号创建时为空）、写 App userId 指纹。
func (s *Service) CollectScanLogin(ctx context.Context, userID int64, ticket string) (*store.Account, error) {
	scanLoginMu.Lock()
	t, ok := scanLoginTickets[ticket]
	if !ok || t.Result == nil {
		scanLoginMu.Unlock()
		return nil, errors.New("没有可应用的登录态，请先扫码")
	}
	if t.UserID != userID {
		scanLoginMu.Unlock()
		return nil, errors.New("票据不属于当前用户")
	}
	result := *t.Result
	delete(scanLoginTickets, ticket)
	scanLoginMu.Unlock()

	account, err := s.db.AccountByID(ctx, userID, t.AccountID)
	if err != nil {
		return nil, err
	}

	// 防绑错：库里已有指纹且与本次不同，说明手机上登录的是另一个人。
	// 这种情况宁可报错让用户自己处理，也不能把别人的身份写进这条记录。
	if existing := strings.TrimSpace(account.AppUserID); existing != "" &&
		result.AppUserID != "" && existing != result.AppUserID {
		return nil, errors.New("该账号上次绑定的是另一位今日校园用户，请确认手机上登录的账号后再试")
	}

	// host 为空时用本次扫码拿到的补上：没有 host 就换不出业务会话。
	host := account.Host
	if cpdaily.NormalizeHost(host) == "" && result.Host != "" {
		host = result.Host
		account.Host = host
		if err := s.db.UpdateAccount(ctx, account); err != nil {
			return nil, err
		}
	}

	// 用票据构造客户端，交给 finishSession 验证并落 cookies：
	// 它会拉一次任务实测会话，失败则标记 expired 并报错。
	client := cpdaily.NewClient(s.opts, map[string]string{"MOD_AUTH_CAS": result.Ticket})
	if _, err := s.finishSession(ctx, account, client); err != nil {
		return nil, err
	}

	if result.AppUserID != "" && result.AppUserID != strings.TrimSpace(account.AppUserID) {
		if err := s.db.UpdateAccountAppUserID(ctx, account.ID, result.AppUserID); err != nil {
			return nil, err
		}
	}

	// 扫码时顺带带回来的设备指纹，一并落库（用户扫一次码就补齐两样东西）。
	if err := s.applyScannedDevice(ctx, userID, account, result.Device); err != nil {
		return nil, err
	}

	s.notify(userID, events.TopicAccounts)
	return s.db.AccountByID(ctx, userID, account.ID)
}

// applyScannedDevice 把扫码页顺带采集到的设备信息写进账号。
//
// 与 CollectDeviceInfo 保持同一套「只在取到好值时覆盖」的策略：
// 机型/版本/构建号为空或 unknown 时不覆盖，平台只在系统名可识别时才改，
// 否则会把 iOS 账号静默改成 android。
func (s *Service) applyScannedDevice(ctx context.Context, userID int64, account *store.Account, info DeviceInfo) error {
	if strings.TrimSpace(info.DeviceID) == "" {
		return nil
	}
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
		appVersion = v
	}
	platform := account.Platform
	switch strings.ToLower(strings.TrimSpace(info.SystemName)) {
	case "ios", "iphone", "ipad":
		platform = "ios"
	case "android":
		platform = "android"
	}
	return s.db.UpdateAccountDevice(ctx, userID, account.ID, store.AccountDevice{
		DeviceID: strings.TrimSpace(info.DeviceID), DeviceModel: deviceModel,
		SystemVersion: systemVersion, Platform: platform, WisDeviceID: wisDeviceID,
		UserAgent: userAgent, AppVersion: appVersion, BuildID: buildID,
	})
}
