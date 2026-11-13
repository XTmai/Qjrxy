package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/events"
	"qjrxy/internal/secure"
)

// 账号导入导出。
//
// 导出默认带上登录态（cookies、tgc），这样换机/重装后导入即可继续使用，
// 不必再收短信或重输密码。密码默认不导出，需显式开启。
// 导出文件属于敏感物料：拿到它等于拿到账号，前端会做二次确认。

// ExportAccount 是导出文件里的一条账号。
type ExportAccount struct {
	Alias         string  `json:"alias"`
	SchoolName    string  `json:"schoolName"`
	Username      string  `json:"username"`
	DeviceID      string  `json:"deviceId"`
	DeviceModel   string  `json:"deviceModel"`
	SystemVersion string  `json:"systemVersion"`
	Platform      string  `json:"platform"`
	WisDeviceID   string  `json:"wisDeviceId"`
	UserAgent     string  `json:"userAgent"`
	AppVersion    string  `json:"appVersion"`
	BuildID       string  `json:"buildId"`
	Address       string  `json:"address"`
	Lon           float64 `json:"lon"`
	Lat           float64 `json:"lat"`
	Abnormal      string  `json:"abnormalReason"`
	LoginStatus   string  `json:"loginStatus"`
	LastLoginAt   string  `json:"lastLoginAt"`

	// Cookies 是业务会话（MOD_AUTH_CAS / sessionToken / tenantId ...），
	// 明文 JSON，跨机可迁移；tgc 用于票过期后自动续期，导出时已解密。
	Cookies map[string]string `json:"cookies,omitempty"`
	TGC     string            `json:"tgc,omitempty"`
	// Password 仅在显式要求时导出（明文）。
	Password string `json:"password,omitempty"`
}

// ExportFile 是 JSON 导出的顶层结构。
type ExportFile struct {
	Version      int             `json:"version"`
	ExportedAt   string          `json:"exportedAt"`
	WithSession  bool            `json:"withSession"`
	WithPassword bool            `json:"withPassword"`
	Accounts     []ExportAccount `json:"accounts"`
	// Warnings 记录导出时没能带上的数据：TGC/密码解密失败会被跳过，
	// 不留痕的话文件头仍宣称 withSession/withPassword，用户拿到的是
	// 一份「看起来带登录态、实际缺凭据」的迁移文件。
	Warnings []string `json:"warnings,omitempty"`
}

// ImportResult 是导入的逐条结果。
type ImportResult struct {
	Created int          `json:"created"`
	Updated int          `json:"updated"`
	Failed  int          `json:"failed"`
	Items   []ImportItem `json:"items"`
}

// ImportItem 是一条导入结果。
type ImportItem struct {
	Username string `json:"username"`
	School   string `json:"school"`
	Status   string `json:"status"` // created / updated / failed
	Error    string `json:"error,omitempty"`
}

// ExportAccounts 组装导出数据。
func (s *Service) ExportAccounts(ctx context.Context, userID int64, withSession, withPassword bool) (*ExportFile, error) {
	list, err := s.db.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &ExportFile{
		Version:      1,
		ExportedAt:   time.Now().Format(time.RFC3339),
		WithSession:  withSession,
		WithPassword: withPassword,
		Accounts:     make([]ExportAccount, 0, len(list)),
	}
	var warnings []string
	for i := range list {
		a := &list[i]
		item := ExportAccount{
			Alias: a.Alias, SchoolName: a.SchoolName, Username: a.Username,
			DeviceID: a.DeviceID, DeviceModel: a.DeviceModel, SystemVersion: a.SystemVersion,
			Platform: a.Platform, WisDeviceID: a.WisDeviceID, UserAgent: a.UserAgent,
			AppVersion: a.AppVersion, BuildID: a.BuildID,
			Address: a.Address, Lon: a.Lon, Lat: a.Lat, Abnormal: a.Abnormal,
			LoginStatus: a.LoginStatus, LastLoginAt: a.LastLoginAt,
		}
		if withSession {
			item.Cookies = decodeCookies(a.Cookies)
			if a.TGCEnc != "" {
				if tgc, derr := secure.Decrypt(s.cfg.SecretKey, a.TGCEnc); derr == nil {
					item.TGC = tgc
				} else {
					warnings = append(warnings,
						fmt.Sprintf("账号 %s：续期票据解密失败，导出文件不含 TGC，导入后无法自动续期", a.Username))
				}
			}
		}
		if withPassword && a.PasswordEnc != "" {
			if pwd, derr := secure.Decrypt(s.cfg.SecretKey, a.PasswordEnc); derr == nil {
				item.Password = pwd
			} else {
				warnings = append(warnings,
					fmt.Sprintf("账号 %s：密码解密失败，导出文件不含密码，导入后无法自动重登", a.Username))
			}
		}
		out.Accounts = append(out.Accounts, item)
	}
	out.Warnings = warnings
	return out, nil
}

// ImportAccounts 逐条导入。单条失败不中断，返回逐条结果。
//
// 复用 CreateAccount 的「同校同学号去重 + 合并」：已存在的账号会被更新
// （updated），不会因为重复导入而报错。
func (s *Service) ImportAccounts(ctx context.Context, userID int64, items []ExportAccount) *ImportResult {
	res := &ImportResult{Items: make([]ImportItem, 0, len(items))}
	for i := range items {
		item := &items[i]
		status, err := s.importOne(ctx, userID, item)
		line := ImportItem{Username: item.Username, School: item.SchoolName, Status: status}
		if err != nil {
			if status != "" {
				// 账号本体已入库、后续步骤（写登录态/设备）才失败：
				// 如实记 partial 并计入 updated，不能报成「没导进来」。
				line.Status = "partial"
				line.Error = err.Error()
				res.Updated++
			} else {
				line.Status = "failed"
				line.Error = err.Error()
				res.Failed++
			}
		} else if status == "updated" {
			res.Updated++
		} else {
			res.Created++
		}
		res.Items = append(res.Items, line)
	}
	if res.Created+res.Updated > 0 {
		s.notify(userID, events.TopicAccounts)
	}
	return res
}

func (s *Service) importOne(ctx context.Context, userID int64, item *ExportAccount) (string, error) {
	if strings.TrimSpace(item.SchoolName) == "" {
		return "", errors.New("缺少学校名称")
	}
	if strings.TrimSpace(item.Username) == "" {
		return "", errors.New("缺少学号")
	}
	in := AccountInput{
		Alias: item.Alias, SchoolName: item.SchoolName, Username: item.Username,
		Password: item.Password,
		DeviceID: item.DeviceID, DeviceModel: item.DeviceModel, SystemVersion: item.SystemVersion,
		Platform: item.Platform, WisDeviceID: item.WisDeviceID, UserAgent: item.UserAgent,
		AppVersion: item.AppVersion, BuildID: item.BuildID,
		Address: item.Address, Lon: item.Lon, Lat: item.Lat, Abnormal: item.Abnormal,
	}
	account, existed, err := s.CreateAccount(ctx, userID, in)
	if err != nil {
		return "", err
	}
	status := "created"
	if existed {
		status = "updated"
	}

	// 登录态：cookies 明文可直写；tgc 用目标机的 secret.key 重新加密。
	if len(item.Cookies) > 0 {
		loginStatus := strings.TrimSpace(item.LoginStatus)
		if loginStatus == "" {
			loginStatus = "unknown"
		}
		if err := s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(item.Cookies), loginStatus); err != nil {
			return status, fmt.Errorf("写入登录态失败：%w", err)
		}
	}
	if strings.TrimSpace(item.TGC) != "" {
		enc, err := secure.Encrypt(s.cfg.SecretKey, strings.TrimSpace(item.TGC))
		if err != nil {
			return status, fmt.Errorf("保存续期票据失败：%w", err)
		}
		if err := s.db.UpdateAccountTGC(ctx, account.ID, enc); err != nil {
			return status, fmt.Errorf("保存续期票据失败：%w", err)
		}
	}
	return status, nil
}

// resolveTenantCached 按学校名解析租户，进程内缓存。
//
// 批量导入时同一个学校会出现几十次，每次走一遍远端学校列表接口既慢又容易
// 触发限流；学校入口地址几乎不变，缓存到进程重启足够。
func (s *Service) resolveTenantCached(ctx context.Context, schoolName string) (*cpdaily.Tenant, error) {
	key := strings.TrimSpace(schoolName)
	s.tenantMu.Lock()
	if t, ok := s.tenantCache[key]; ok {
		s.tenantMu.Unlock()
		return t, nil
	}
	s.tenantMu.Unlock()

	tenant, err := cpdaily.ResolveTenant(ctx, schoolName)
	if err != nil {
		return nil, err
	}
	s.tenantMu.Lock()
	if s.tenantCache == nil {
		s.tenantCache = map[string]*cpdaily.Tenant{}
	}
	s.tenantCache[key] = tenant
	s.tenantMu.Unlock()
	return tenant, nil
}
