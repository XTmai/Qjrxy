package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"qingjin/internal/cpdaily"
	"qingjin/internal/secure"
	"qingjin/internal/store"
)

// LoginTicket 是一次"待输入验证码"的登录。
type LoginTicket struct {
	ID          string
	AccountID   int64
	NeedCaptcha bool
	Captcha     []byte
	CaptchaKind string
	ExpiresAt   time.Time
}

// BeginLogin 用账号保存的密码发起登录。
//
// 返回的 ticket 表示需要验证码；若账号无需验证码，ticket.ID 为空且登录已完成。
func (s *Service) BeginLogin(ctx context.Context, userID, accountID int64) (*LoginTicket, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	password, err := secure.Decrypt(s.cfg.SecretKey, account.PasswordEnc)
	if err != nil {
		return nil, errors.New("账号密码解析失败，请重新填写密码")
	}
	if password == "" {
		return nil, errors.New("该账号未保存密码，请改用 Cookie 导入")
	}

	tenant := &cpdaily.Tenant{
		ID: account.TenantID, Name: account.SchoolName, JoinType: account.JoinType,
		Host: account.Host, IDSURL: account.IDSURL,
	}
	if tenant.Host == "" || tenant.IDSURL == "" {
		resolved, err := cpdaily.ResolveTenant(ctx, account.SchoolName)
		if err != nil {
			return nil, err
		}
		tenant = resolved
		account.TenantID, account.Host, account.IDSURL = resolved.ID, resolved.Host, resolved.IDSURL
		account.JoinType = resolved.JoinType
	}

	challenge, client, err := cpdaily.StartLogin(ctx, s.opts, tenant, account.Username, password)
	if err != nil {
		_ = s.db.UpdateAccountSession(ctx, account.ID, account.Cookies, "expired")
		return nil, err
	}
	_ = s.db.UpdateAccountSession(ctx, account.ID, account.Cookies, account.LoginStatus)

	if challenge.Done {
		return s.finishSession(ctx, account, client)
	}
	ticket := secure.RandomID(18)
	s.mu.Lock()
	// 清理过期状态
	for id, state := range s.state {
		if time.Since(state.CreatedAt) > 10*time.Minute {
			delete(s.state, id)
		}
	}
	challenge.State.Cookies = client.Cookies()
	s.state[ticket] = challenge.State
	s.mu.Unlock()

	return &LoginTicket{
		ID:          ticket,
		AccountID:   account.ID,
		NeedCaptcha: challenge.NeedCaptcha,
		Captcha:     challenge.Captcha,
		CaptchaKind: challenge.CaptchaKind,
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}, nil
}

// SubmitCaptcha 提交验证码完成登录。
func (s *Service) SubmitCaptcha(ctx context.Context, userID, accountID int64, ticketID, code string) error {
	s.mu.Lock()
	state, ok := s.state[ticketID]
	if ok {
		delete(s.state, ticketID)
	}
	s.mu.Unlock()
	if !ok {
		return errors.New("验证码会话已过期，请重新登录")
	}
	if time.Since(state.CreatedAt) > 10*time.Minute {
		return errors.New("验证码会话已过期，请重新登录")
	}
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return err
	}
	// 用登录页拿到的 cookie 重建客户端，保证 lt / captcha 会话一致。
	client := cpdaily.NewClient(s.opts, state.Cookies)
	done, err := cpdaily.FinishLoginWith(ctx, s.opts, state, code, client)
	if err != nil {
		return err
	}
	_, err = s.finishSession(ctx, account, done)
	return err
}

// finishSession 保存 cookie 并验证会话可用。
func (s *Service) finishSession(ctx context.Context, account *store.Account, client *cpdaily.Client) (*LoginTicket, error) {
	cookies := client.Cookies()
	encoded := encodeCookies(cookies)
	if len(cookies) == 0 {
		return nil, errors.New("登录未返回会话，请改用 Cookie 导入")
	}
	status := "authorized"
	// 用一次任务查询验证会话是否真正可用。
	if _, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindSign}); err != nil {
		if cpdaily.IsAuthExpired(err) {
			status = "expired"
		}
	}
	if err := s.db.UpdateAccountSession(ctx, account.ID, encoded, status); err != nil {
		return nil, err
	}
	return &LoginTicket{ID: "", AccountID: account.ID}, nil
}

// VerifySession 用已保存的 cookie 拉一次任务，判断登录态是否有效。
func (s *Service) VerifySession(ctx context.Context, userID, accountID int64) (string, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return "", err
	}
	if !account.HasCookie() {
		return "unknown", errors.New("尚未登录，请先登录或导入 Cookie")
	}
	client := s.clientFor(account)
	if _, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindSign}); err != nil {
		if cpdaily.IsAuthExpired(err) {
			_ = s.db.UpdateAccountSession(ctx, account.ID, account.Cookies, "expired")
			return "expired", nil
		}
		return account.LoginStatus, err
	}
	_ = s.db.UpdateAccountSession(ctx, account.ID, account.Cookies, "authorized")
	return "authorized", nil
}

// ImportCookies 直接导入浏览器 Cookie。
func (s *Service) ImportCookies(ctx context.Context, userID, accountID int64, raw string) (string, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return "", err
	}
	cookies := cpdaily.ImportCookieString(raw)
	if len(cookies) == 0 {
		return "", errors.New("Cookie 格式不正确，应为 a=1; b=2 形式")
	}
	client := cpdaily.NewClient(s.opts, cookies)
	status := "authorized"
	if _, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindSign}); err != nil {
		if cpdaily.IsAuthExpired(err) {
			status = "expired"
		}
	}
	if err := s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(cookies), status); err != nil {
		return "", err
	}
	if status == "expired" {
		return status, errors.New("Cookie 已失效，请重新复制")
	}
	return status, nil
}

// CaptchaDataURL 把验证码图片转成 data URL。
func CaptchaDataURL(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

// SyncTasks 拉取账号三类任务的待办列表并写入缓存。
func (s *Service) SyncTasks(ctx context.Context, userID, accountID int64) ([]store.Task, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if !account.HasCookie() {
		return nil, errors.New("账号尚未登录，请先登录或导入 Cookie")
	}
	client := s.clientFor(account)
	kinds := []cpdaily.TaskKind{cpdaily.KindAttendance, cpdaily.KindSign, cpdaily.KindCollect}
	tasks, err := client.ListAllTasks(ctx, account.Host, kinds)
	if err != nil {
		if cpdaily.IsAuthExpired(err) {
			_ = s.db.UpdateAccountSession(ctx, account.ID, account.Cookies, "expired")
		}
		return nil, fmt.Errorf("拉取任务失败：%w", err)
	}

	cached := make([]store.Task, 0, len(tasks))
	for _, t := range tasks {
		cached = append(cached, store.Task{
			AccountID:       account.ID,
			Kind:            string(t.Kind),
			SignInstanceWID: t.SignInstanceWID,
			SignWID:         t.SignWID,
			TaskName:        t.TaskName,
			Sender:          t.Sender,
			StartTime:       t.StartTime,
			EndTime:         t.EndTime,
			Status:          t.Status,
			StuSignWID:      t.StuSignWID,
			SignDate:        t.SignDate,
			BeginClock:      t.BeginClock,
			EndClock:        t.EndClock,
			SignStatus:      t.SignStatus,
			SignStatusText:  t.SignStatusText,
		})
	}
	if err := s.db.ReplaceTasks(ctx, account.ID, cached); err != nil {
		return nil, err
	}
	return s.db.TasksByAccount(ctx, account.ID)
}
