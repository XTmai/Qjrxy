package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/events"
	"qjrxy/internal/secure"
	"qjrxy/internal/store"
)

const (
	paramSMSPhone   = "_smsPhone"
	paramSMSProfile = "_smsProfile"
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
		return nil, errors.New("该账号未保存密码，请先在账号里填写密码")
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
		// 登录失败会把账号标成 expired，账号页的「需重新登录」标签应立刻出现。
		s.notify(userID, events.TopicAccounts)
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
		if time.Since(state.CreatedAt) > loginStateTTL {
			delete(s.state, id)
		}
	}
	challenge.State.Cookies = client.Cookies()
	// 绑定发起者，供 SubmitCaptcha 校验 ticket 归属。
	challenge.State.UserID = userID
	challenge.State.AccountID = account.ID
	s.state[ticket] = challenge.State
	s.mu.Unlock()

	return &LoginTicket{
		ID:          ticket,
		AccountID:   account.ID,
		NeedCaptcha: challenge.NeedCaptcha,
		Captcha:     challenge.Captcha,
		CaptchaKind: challenge.CaptchaKind,
		ExpiresAt:   time.Now().Add(loginStateTTL),
	}, nil
}

// SMSTicket 是一次「已发送短信、等待验证码」的登录。
type SMSTicket struct {
	ID        string    `json:"ticket"`
	Phone     string    `json:"phone"` // 掩码后的手机号，仅用于界面回显
	ExpiresAt time.Time `json:"expiresAt"`
}

// BeginSMSLogin 用今日校园账号绑定的手机号发送验证码。
//
// 走 campushoy 官方账号体系（mobile.campushoy.com），登录成功后由服务端
// 返回学校 CAS 票（tgc），再用它换取业务域会话——因此所有学校通用，
// 不依赖学校网页是否开放短信入口。
func (s *Service) BeginSMSLogin(ctx context.Context, userID, accountID int64, phone string) (*SMSTicket, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil, errors.New("请填写手机号")
	}
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	device := deviceProfile(account, account.Lon, account.Lat)
	tenantID := strings.TrimSpace(account.TenantID)
	if tenantID == "" || cpdaily.NormalizeHost(account.Host) == "" {
		// 老数据可能没存租户信息，用学校名重新解析一次并写回：
		// tenantId/host 之后要用于发码头、换票入口和任务接口，只改局部变量
		// 会让下一次登录又回到同样的缺失状态。
		if tenant, terr := cpdaily.ResolveTenant(ctx, account.SchoolName); terr == nil {
			tenantID = tenant.ID
			account.TenantID = tenant.ID
			account.Host = tenant.Host
			account.IDSURL = tenant.IDSURL
			account.JoinType = tenant.JoinType
			if account.SchoolName == "" {
				account.SchoolName = tenant.Name
			}
			if err := s.db.UpdateAccount(ctx, account); err != nil {
				return nil, err
			}
		}
	}
	if err := cpdaily.SendAppSMSCode(ctx, device, tenantID, phone); err != nil {
		return nil, err
	}

	ticket := secure.RandomID(18)
	s.mu.Lock()
	for id, st := range s.state {
		if st == nil || time.Since(st.CreatedAt) > loginStateTTL {
			delete(s.state, id)
		}
	}
	// 复用登录中间态：只存手机号与设备信息，不含密码
	state := &cpdaily.LoginState{
		Tenant: &cpdaily.Tenant{
			ID: account.TenantID, Name: account.SchoolName, JoinType: account.JoinType,
			Host: account.Host, IDSURL: account.IDSURL,
		},
		Username:  phone,
		UserID:    userID,
		AccountID: account.ID,
		Params:    map[string]string{paramSMSPhone: phone},
		CreatedAt: time.Now(),
	}
	s.state[ticket] = state
	s.mu.Unlock()

	return &SMSTicket{
		ID:        ticket,
		Phone:     maskPhone(phone),
		ExpiresAt: time.Now().Add(loginStateTTL),
	}, nil
}

// SubmitSMSLogin 用验证码完成 App 登录，并用返回的 CAS 票换取业务会话。
func (s *Service) SubmitSMSLogin(ctx context.Context, userID, accountID int64, ticketID, code string) (*store.Account, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("请输入验证码")
	}
	s.mu.Lock()
	state, ok := s.state[ticketID]
	if ok {
		delete(s.state, ticketID)
	}
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("验证码会话已过期，请重新获取")
	}
	if time.Since(state.CreatedAt) > loginStateTTL {
		return nil, errors.New("验证码会话已过期，请重新获取")
	}
	if state.UserID != userID || state.AccountID != accountID {
		return nil, errors.New("验证码会话与当前账号不匹配，请重新获取")
	}
	phone := state.Params[paramSMSPhone]

	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	device := deviceProfile(account, account.Lon, account.Lat)
	result, err := cpdaily.AppMobileLogin(ctx, device, account.TenantID, phone, code)
	if err != nil {
		return nil, err
	}
	// 一行诊断：线上反馈基本说不清是「该校不发票」还是「租户没解析出来」，
	// 这行日志（不含手机号/学号/票值）能把两类原因分开。
	log.Printf("短信登录 %s", describeLoginResult(result))

	// 登录响应里的 tenantId 才是这次登录真正落在的租户；账号上存的是创建时的，
	// 可能是空的或旧的（学校合并/换域名）。以响应为准反查门户域名，比用学校名模糊
	// 匹配可靠——「缺少换票参数」的反馈基本都出在这一步。
	tenantChanged := result.TenantID != "" && result.TenantID != account.TenantID
	if tenantChanged {
		account.TenantID = result.TenantID
	}
	host := ""
	if !tenantChanged {
		host = cpdaily.NormalizeHost(account.Host)
	}
	if host == "" && result.TenantID != "" {
		if tenant, terr := cpdaily.ResolveTenantByID(ctx, result.TenantID); terr == nil {
			host = tenant.Host
			if account.SchoolName == "" {
				account.SchoolName = tenant.Name
			}
		}
	}
	if host == "" {
		host = cpdaily.NormalizeHost(account.Host)
	}
	if host == "" && account.SchoolName != "" {
		if tenant, terr := cpdaily.ResolveTenant(ctx, account.SchoolName); terr == nil {
			host = tenant.Host
		}
	}
	if host == "" {
		return nil, errors.New("未识别到学校门户域名，请确认账号所选的学校是否正确（学校名需与今日校园里一致）")
	}
	if host != cpdaily.NormalizeHost(account.Host) || account.SchoolName == "" {
		account.Host = host
		if err := s.db.UpdateAccount(ctx, account); err != nil {
			return nil, err
		}
	}

	client := cpdaily.NewClient(s.opts, nil)
	// App 登录成功后本来就是带 sessionToken 的，部分学校（不下发 CAS 票）只有它。
	// 先种进去，任务接口的 sessionToken/sessionTokenKey 才拿得到值。
	if token := strings.TrimSpace(result.SessionToken); token != "" {
		client.SetCookies(map[string]string{
			"sessionToken": token,
			"tenantId":     account.TenantID,
		})
	}

	// tgc 为空说明该校不走 IAP CAS 换票（账号体系会话直接可用），
	// 这时不该拦在「缺少换票参数」上，交给 finishSession 实测会话是否有效。
	tgc := strings.TrimSpace(result.TGC)
	var exchangeErr error
	if tgc != "" {
		exchangeErr = cpdaily.ExchangeTGC(ctx, client, host, tgc, device, account.TenantID)
	}

	if _, err := s.finishSession(ctx, account, client); err != nil {
		if exchangeErr != nil {
			return nil, exchangeErr
		}
		return nil, err
	}
	fresh, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	// 换票失败但会话实测可用 → 照常算登录成功（部分学校业务域不需要 MOD_AUTH_CAS）。
	if exchangeErr != nil && fresh.LoginStatus != "authorized" {
		return nil, exchangeErr
	}
	// 没有票据、sessionToken 也访问不了业务接口：这次登录实际是失败的，
	// 不能因为 finishSession 已经把 cookies 落库就报成功。
	if tgc == "" && fresh.LoginStatus != "authorized" {
		return nil, errors.New("该校登录未下发换票票据，且账号会话无法访问业务接口；请改用扫码授权或账密登录")
	}
	return fresh, nil
}

// describeLoginResult 把登录响应的关键状态拼成一行，供日志排查使用（不含票值）。
func describeLoginResult(res *cpdaily.AppLoginResult) string {
	pick := func(v string, ok bool) string {
		if ok {
			return "有"
		}
		return "无"
	}
	return fmt.Sprintf("tenant=%s status=%s tgcStatus=%s tgc=%s session=%s",
		res.TenantID, res.Status, res.TGCStatus,
		pick(res.TGC, strings.TrimSpace(res.TGC) != ""),
		pick(res.SessionToken, strings.TrimSpace(res.SessionToken) != ""))
}

// maskPhone 保留前 3 后 2 位。
func maskPhone(p string) string {
	r := []rune(p)
	if len(r) <= 5 {
		return p
	}
	return string(r[:3]) + "****" + string(r[len(r)-2:])
}

// loginStateTTL 是登录中间态（含明文密码）的最长保留时间。
const loginStateTTL = 10 * time.Minute

// PruneExpiredStates 主动回收过期的登录中间态。
//
// LoginState 里保存着明文密码（CAS/IAP 表单需要重放），不能只等下一次
// BeginLogin 才顺手清理——没人再登录时，这些状态会一直留在内存里。
// 由调度器每分钟调用一次。
func (s *Service) PruneExpiredStates() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, state := range s.state {
		if state == nil || time.Since(state.CreatedAt) > loginStateTTL {
			delete(s.state, id)
			removed++
		}
	}
	return removed
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
	if time.Since(state.CreatedAt) > loginStateTTL {
		return errors.New("验证码会话已过期，请重新登录")
	}
	// 校验 ticket 归属：只能完成自己发起的登录，否则会把别人的会话写进当前账号。
	if state.UserID != userID || state.AccountID != accountID {
		return errors.New("验证码会话与当前账号不匹配，请重新登录")
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
	if len(client.Cookies()) == 0 {
		return nil, errors.New("登录未返回会话，请稍后重试")
	}
	status := "authorized"
	// 用一次任务查询验证会话是否真正可用。
	if _, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindSign}); err != nil {
		if cpdaily.IsAuthExpired(err) {
			status = "expired"
		}
	}
	// 验证请求本身也可能带回 Set-Cookie（续期），所以快照要取在验证之后；
	// 并且与旧 cookie 合并，保留本次登录没有下发但可续期的那张 CAS 票。
	encoded := encodeCookies(mergeCookies(account.Cookies, client.Cookies()))
	if err := s.db.UpdateAccountSession(ctx, account.ID, encoded, status); err != nil {
		return nil, err
	}
	s.notify(account.UserID, events.TopicAccounts)
	return &LoginTicket{ID: "", AccountID: account.ID}, nil
}

// VerifySession 用已保存的 cookie 拉一次任务，判断登录态是否有效。
func (s *Service) VerifySession(ctx context.Context, userID, accountID int64) (string, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return "", err
	}
	if !account.HasCookie() {
		return "unknown", errors.New("账号尚未登录，请先完成账密登录")
	}
	client := s.clientFor(account)
	if _, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindSign}); err != nil {
		if cpdaily.IsAuthExpired(err) {
			// 存 client 的快照而不是 account.Cookies：这一轮请求可能已经
			// 带回续期用的 Set-Cookie，存旧值等于把续期丢掉。
			_ = s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(client.Cookies()), "expired")
			s.notify(userID, events.TopicAccounts)
			return "expired", nil
		}
		return account.LoginStatus, err
	}
	_ = s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(client.Cookies()), "authorized")
	s.notify(userID, events.TopicAccounts)
	return "authorized", nil
}

// describeAttendanceWindow 用一句话描述该账号当前的查寝任务状态。
//
// 比「同步到 N 个任务」有用得多：用户真正想知道的是「现在要不要管它」。
func describeAttendanceWindow(s *Service, ctx context.Context, accountID int64) string {
	w, ok := s.NextTaskWindow(ctx, accountID, string(cpdaily.KindAttendance))
	if !ok {
		return "今日暂无查寝任务"
	}
	if w.Ready {
		return "有查寝任务可执行"
	}
	now := time.Now()
	// 同一天只显示时刻，跨天补上日期
	if w.Start.Format("2006-01-02") == now.Format("2006-01-02") {
		return fmt.Sprintf("查寝 %s 开始", w.Start.Format("15:04"))
	}
	return fmt.Sprintf("查寝 %s 开始", w.Start.Format("01-02 15:04"))
}

// ProbeResult 是一次账号登录态探测的结果。
type ProbeResult struct {
	AccountID   int64  `json:"accountId"`
	AccountName string `json:"accountName"`
	// Status 取值：authorized（正常）/ relogged（已自动重登恢复）/ expired（需要人工登录）
	Status    string `json:"status"`
	Message   string `json:"message"`
	TaskCount int    `json:"taskCount"`
	Relogged  bool   `json:"relogged"`
	Error     string `json:"error,omitempty"`
}

// ProbeAccount 探测单个账号的登录态，并在会话失效时自动重新登录、同步任务。
//
// 这是「自动拉取任务」的核心：
//  1. 用现有会话拉一次任务列表，成功即同步任务并更新登录态
//  2. 会话失效 → 用保存的密码自动重登 → 重试拉取 → 写回新会话
//  3. 没有密码或重登失败 → 标记 expired，等用户手动处理
//
// 全程不会因为某一步失败而中断其他账号的探测。
func (s *Service) ProbeAccount(ctx context.Context, account *store.Account) ProbeResult {
	res := ProbeResult{AccountID: account.ID, AccountName: account.Alias}
	// 每条探测分支都会写登录态/探测时间，统一在返回时通知一次，
	// 免得在每个 return 前重复加。参数在 defer 时求值，账号指针后续被
	// 替换也不影响用户归属。
	defer s.notify(account.UserID, events.TopicAccounts)

	// 情况一：从未登录过 / 已标记失效 → 直接尝试自动登录
	if !account.HasCookie() || strings.EqualFold(account.LoginStatus, "expired") {
		if !hasCredentials(account) {
			res.Status = "expired"
			res.Message = "尚未登录，且未保存密码，请手动完成账密登录"
			_ = s.db.RecordProbe(ctx, account.ID, "expired", res.Message)
			return res
		}
		updated, err := s.AutoRelogin(ctx, account)
		if err != nil {
			res.Status = "expired"
			res.Message = "自动登录失败，请手动登录"
			res.Error = err.Error()
			_ = s.db.RecordProbe(ctx, account.ID, "expired", res.Message)
			return res
		}
		account = updated
		res.Relogged = true
	}

	// 用当前会话拉任务；失效则重登后重试一次
	tasks, err := s.SyncTasks(ctx, account.UserID, account.ID)
	if err != nil && cpdaily.IsAuthExpired(err) && hasCredentials(account) && !res.Relogged {
		if updated, reErr := s.AutoRelogin(ctx, account); reErr == nil {
			account = updated
			res.Relogged = true
			tasks, err = s.SyncTasks(ctx, account.UserID, account.ID)
		}
	}

	if err != nil {
		status, msg := "expired", "登录态失效，需要重新登录"
		if !cpdaily.IsAuthExpired(err) {
			// 非会话问题（网络、服务端异常）不该把账号标记成需要登录
			status = account.LoginStatus
			if status == "" || status == "unknown" {
				status = "authorized"
			}
			msg = "探测失败：" + err.Error()
		}
		res.Status = status
		res.Message = msg
		res.Error = err.Error()
		_ = s.db.RecordProbe(ctx, account.ID, status, res.Message)
		return res
	}

	// 只统计查寝任务：本项目只处理查寝，把签到/信息收集也算进来会让用户
	// 误以为有一堆待打卡任务（实际那两类根本不会执行）。
	attendance := 0
	for i := range tasks {
		if tasks[i].Kind == string(cpdaily.KindAttendance) {
			attendance++
		}
	}
	res.TaskCount = attendance
	detail := describeAttendanceWindow(s, ctx, account.ID)
	prefix := "登录态正常"
	if res.Relogged {
		res.Status = "relogged"
		prefix = "会话已自动恢复"
	} else {
		res.Status = "authorized"
	}
	res.Message = prefix + "，" + detail
	_ = s.db.RecordProbe(ctx, account.ID, "authorized", res.Message)
	return res
}

// ProbeAllAccounts 探测所有账号（后台定时任务使用）。
func (s *Service) ProbeAllAccounts(ctx context.Context) []ProbeResult {
	accounts, err := s.db.ListAllAccounts(ctx)
	if err != nil {
		return nil
	}
	results := make([]ProbeResult, 0, len(accounts))
	for i := range accounts {
		// 单个账号失败不影响其他账号
		if ctx.Err() != nil {
			break
		}
		results = append(results, s.ProbeAccount(ctx, &accounts[i]))
	}
	return results
}

// hasCredentials 判断账号是否保存了可自动登录的密码。
func hasCredentials(a *store.Account) bool {
	return strings.TrimSpace(a.PasswordEnc) != ""
}

// AutoRelogin 用账号保存的密码自动重新登录，用于会话过期后的无感恢复。
//
// 成功返回刷新后的账号；学校要求图形验证码时返回错误，
// 这种情况需要用户到账号页手动完成一次登录。
func (s *Service) AutoRelogin(ctx context.Context, account *store.Account) (*store.Account, error) {
	if !hasCredentials(account) {
		return nil, errors.New("账号未保存密码，无法自动重新登录")
	}
	ticket, err := s.BeginLogin(ctx, account.UserID, account.ID)
	if err != nil {
		return nil, err
	}
	if ticket.ID != "" {
		return nil, errors.New("本次登录需要图形验证码，请到账号页手动登录一次")
	}
	return s.db.AccountByID(ctx, account.UserID, account.ID)
}

// CaptchaDataURL 把验证码图片转成 data URL。
func CaptchaDataURL(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

// SyncTasks 拉取账号三类任务的待办列表并写入缓存。
//
// 会话失效时自动用保存的密码重新登录一次再重试，用户无需手动操作。
func (s *Service) SyncTasks(ctx context.Context, userID, accountID int64) ([]store.Task, error) {
	account, err := s.db.AccountByID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if !account.HasCookie() || strings.EqualFold(account.LoginStatus, "expired") {
		if !hasCredentials(account) {
			return nil, errors.New("账号尚未登录，请先完成账密登录")
		}
		if account, err = s.AutoRelogin(ctx, account); err != nil {
			return nil, fmt.Errorf("自动登录失败：%w", err)
		}
	}
	client := s.clientFor(account)
	kinds := []cpdaily.TaskKind{cpdaily.KindAttendance, cpdaily.KindSign, cpdaily.KindCollect}
	tasks, err := client.ListAllTasks(ctx, account.Host, kinds)
	if err != nil && cpdaily.IsAuthExpired(err) && hasCredentials(account) {
		// 提交时才发现会话过期：重登一次再试。
		if updated, reErr := s.AutoRelogin(ctx, account); reErr == nil {
			account = updated
			client = s.clientFor(account)
			tasks, err = client.ListAllTasks(ctx, account.Host, kinds)
		}
	}
	if err != nil {
		if cpdaily.IsAuthExpired(err) {
			_ = s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(client.Cookies()), "expired")
			s.notify(userID, events.TopicAccounts)
			return nil, fmt.Errorf("拉取任务失败：%w", err)
		}
		// 部分成功：某一类任务失败（如学校未开通信息收集），但其余已拿到。
		// 本项目只用查寝，只要查寝那一类没整体失败就继续写缓存，
		// 否则会因无关类型的错误把可用任务整批丢掉。
		hasAttendance := false
		for _, t := range tasks {
			if t.Kind == cpdaily.KindAttendance {
				hasAttendance = true
				break
			}
		}
		if !hasAttendance && !strings.Contains(err.Error(), "查寝") {
			return nil, fmt.Errorf("拉取任务失败：%w", err)
		}
	}

	// 这一轮请求可能带回了续期 cookie（服务端若做滑动续期），成功时落库，
	// 免得下次 clientFor 又从 DB 重建出旧票。
	_ = s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(client.Cookies()), "authorized")

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
	s.notify(userID, events.TopicTasks)
	return s.db.TasksByAccount(ctx, account.ID)
}
