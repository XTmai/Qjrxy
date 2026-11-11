package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/config"
	"qjrxy/internal/cpdaily"
	"qjrxy/internal/secure"
	"qjrxy/internal/store"
)

// Service 聚合业务逻辑，供 HTTP 层与调度器共用。
type Service struct {
	cfg  *config.Config
	db   *store.DB
	opts cpdaily.ClientOptions

	mu    sync.RWMutex
	state map[string]*cpdaily.LoginState // loginTicket -> 登录中间态
}

// New 创建服务。
func New(cfg *config.Config, db *store.DB) *Service {
	photos := cfg.PhotosDir()
	_ = os.MkdirAll(photos, 0o755)
	// 证书校验策略：默认对学校系统做完整校验，只有配置里显式列出的主机才跳过。
	// 这条链路承载学号与密码，不能再按早期版本全局关闭校验。
	cpdaily.ConfigureTLSInsecureHosts(cfg.TLSInsecureHosts)
	return &Service{
		cfg:   cfg,
		db:    db,
		state: map[string]*cpdaily.LoginState{},
		opts: cpdaily.ClientOptions{
			Timeout:       40 * time.Second,
			InsecureHosts: cfg.TLSInsecureHosts,
		},
	}
}

// DB 暴露存储层。
func (s *Service) DB() *store.DB { return s.db }

// Config 暴露配置。
func (s *Service) Config() *config.Config { return s.cfg }

// ---------- 认证 ----------

// Register 注册用户。首个用户自动成为管理员并获得较高额度。
func (s *Service) Register(ctx context.Context, username, password, nickname string) (*store.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 {
		return nil, errors.New("用户名至少 3 位")
	}
	if len(password) < 6 {
		return nil, errors.New("密码至少 6 位")
	}
	hash, err := secure.HashPassword(password)
	if err != nil {
		return nil, err
	}
	if nickname == "" {
		nickname = username
	}
	return s.db.CreateUser(ctx, username, nickname, hash, 50)
}

// Login 校验用户名密码并签发会话 token。
func (s *Service) Login(ctx context.Context, username, password string) (*store.User, string, error) {
	user, err := s.db.UserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		return nil, "", errors.New("用户名或密码错误")
	}
	if err != nil {
		return nil, "", err
	}
	if !secure.VerifyPassword(password, user.PasswordHash) {
		return nil, "", errors.New("用户名或密码错误")
	}
	token := secure.NewToken()
	if err := s.db.CreateSession(ctx, token, user.ID, 30*24*time.Hour); err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// Logout 注销会话。
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.db.DeleteSession(ctx, token)
}

// Authenticate 解析会话 token。
func (s *Service) Authenticate(ctx context.Context, token string) (*store.User, error) {
	if token == "" {
		return nil, store.ErrNotFound
	}
	return s.db.SessionUser(ctx, token)
}

// ---------- 学校解析 ----------

// SearchSchools 搜索学校名称，最多返回 limit 条。
func (s *Service) SearchSchools(ctx context.Context, keyword string, limit int) ([]cpdaily.School, error) {
	all, err := cpdaily.ListSchools(ctx)
	if err != nil {
		return nil, err
	}
	keyword = strings.TrimSpace(keyword)
	out := make([]cpdaily.School, 0, 20)
	for _, school := range all {
		if keyword == "" || strings.Contains(school.Name, keyword) {
			out = append(out, school)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// ---------- 账号 ----------

// AccountInput 是新增/编辑账号的输入。
type AccountInput struct {
	Alias         string  `json:"alias"`
	SchoolName    string  `json:"schoolName"`
	Username      string  `json:"username"`
	Password      string  `json:"password"`
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
}

// CreateAccount 解析学校并保存账号（不执行登录）。
//
// 同一学校下学号已存在时不再当成失败：直接复用该账号，把本次填写的密码/别名/
// 设备信息更新进去后返回 existed=true。这样「保存即登录」重试、双击重复提交、
// 或创建成功但紧接着登录失败后用户再点一次，都不会再看到「已绑定」的死胡同，
// 也不会出现「提示已存在、账号确实已添加、但列表要刷新才看得到」的错位。
func (s *Service) CreateAccount(ctx context.Context, userID int64, in AccountInput) (*store.Account, bool, error) {
	if strings.TrimSpace(in.SchoolName) == "" {
		return nil, false, errors.New("请填写学校全称")
	}
	if strings.TrimSpace(in.Username) == "" {
		return nil, false, errors.New("请填写学号")
	}
	tenant, err := cpdaily.ResolveTenant(ctx, in.SchoolName)
	if err != nil {
		return nil, false, err
	}
	passwordEnc, err := secure.Encrypt(s.cfg.SecretKey, in.Password)
	if err != nil {
		return nil, false, err
	}
	username := strings.TrimSpace(in.Username)

	// 先查一次：已绑定则更新密码/设备信息后返回，不报 UNIQUE。
	existing, err := s.db.AccountBySchoolUsername(ctx, userID, tenant.Name, username)
	if err == nil {
		if err := s.rebindAccount(ctx, existing, in, tenant, passwordEnc); err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, false, err
	}

	account := &store.Account{
		UserID:        userID,
		Alias:         firstNonEmpty(in.Alias, in.Username),
		SchoolName:    tenant.Name,
		Username:      username,
		PasswordEnc:   passwordEnc,
		TenantID:      tenant.ID,
		Host:          tenant.Host,
		IDSURL:        tenant.IDSURL,
		JoinType:      defaultIfEmpty(tenant.JoinType, "CAS"),
		DeviceID:      strings.TrimSpace(in.DeviceID),
		DeviceModel:   defaultIfEmpty(in.DeviceModel, "23127PN0CC"),
		SystemVersion: defaultIfEmpty(in.SystemVersion, "14"),
		Platform:      normalizePlatform(in.Platform),
		WisDeviceID:   strings.TrimSpace(in.WisDeviceID),
		UserAgent:     strings.TrimSpace(in.UserAgent),
		AppVersion:    strings.TrimSpace(in.AppVersion),
		BuildID:       defaultIfEmpty(in.BuildID, cpdaily.DefaultBuildID),
		Address:       strings.TrimSpace(in.Address),
		Lon:           in.Lon,
		Lat:           in.Lat,
		Abnormal:      defaultIfEmpty(in.Abnormal, "在校"),
		LoginStatus:   "unknown",
	}
	created, err := s.db.CreateAccount(ctx, account)
	if errors.Is(err, store.ErrDuplicateAccount) {
		// 并发下被另一个请求抢先插入：取回它并按「已存在」处理。
		existing, lookupErr := s.db.AccountBySchoolUsername(ctx, userID, tenant.Name, username)
		if lookupErr != nil {
			return nil, false, err
		}
		if err := s.rebindAccount(ctx, existing, in, tenant, passwordEnc); err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return created, false, nil
}

// rebindAccount 把本次输入合并进已存在的账号并落库。
// 密码只在本次确实填写时覆盖，避免空密码把可用凭据清空。
func (s *Service) rebindAccount(ctx context.Context, a *store.Account, in AccountInput, tenant *cpdaily.Tenant, passwordEnc string) error {
	if v := strings.TrimSpace(in.Alias); v != "" {
		a.Alias = v
	}
	a.Username = strings.TrimSpace(in.Username)
	a.SchoolName = tenant.Name
	a.TenantID = tenant.ID
	a.Host = tenant.Host
	a.IDSURL = tenant.IDSURL
	a.JoinType = defaultIfEmpty(tenant.JoinType, "CAS")
	if strings.TrimSpace(in.Password) != "" {
		a.PasswordEnc = passwordEnc
		a.LoginStatus = "unknown"
	}
	if v := strings.TrimSpace(in.DeviceID); v != "" {
		a.DeviceID = v
	}
	if v := strings.TrimSpace(in.DeviceModel); v != "" {
		a.DeviceModel = v
	}
	if v := strings.TrimSpace(in.SystemVersion); v != "" {
		a.SystemVersion = v
	}
	if v := strings.TrimSpace(in.Platform); v != "" {
		a.Platform = normalizePlatform(v)
	}
	if v := strings.TrimSpace(in.WisDeviceID); v != "" {
		a.WisDeviceID = v
	}
	if v := strings.TrimSpace(in.UserAgent); v != "" {
		a.UserAgent = v
	}
	if v := strings.TrimSpace(in.AppVersion); v != "" {
		a.AppVersion = v
	}
	if v := strings.TrimSpace(in.BuildID); v != "" {
		a.BuildID = v
	}
	if v := strings.TrimSpace(in.Address); v != "" {
		a.Address = v
	}
	if in.Lon != 0 || in.Lat != 0 {
		a.Lon, a.Lat = in.Lon, in.Lat
	}
	if v := strings.TrimSpace(in.Abnormal); v != "" {
		a.Abnormal = v
	}
	return s.db.UpdateAccount(ctx, a)
}

// UpdateAccount 更新账号信息（密码留空表示不变）。
func (s *Service) UpdateAccount(ctx context.Context, userID, id int64, in AccountInput) (*store.Account, error) {
	account, err := s.db.AccountByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if in.Alias != "" {
		account.Alias = in.Alias
	}
	// 学号也允许修改：换绑账号或之前填错时需要更正
	if v := strings.TrimSpace(in.Username); v != "" {
		account.Username = v
	}
	// 学校改动能同步更新租户/入口信息
	if v := strings.TrimSpace(in.SchoolName); v != "" && v != account.SchoolName {
		tenant, err := cpdaily.ResolveTenant(ctx, v)
		if err != nil {
			return nil, err
		}
		account.SchoolName = tenant.Name
		account.TenantID = tenant.ID
		account.Host = tenant.Host
		account.IDSURL = tenant.IDSURL
		account.JoinType = defaultIfEmpty(tenant.JoinType, "CAS")
	}
	if in.Password != "" {
		enc, err := secure.Encrypt(s.cfg.SecretKey, in.Password)
		if err != nil {
			return nil, err
		}
		account.PasswordEnc = enc
	}
	if in.DeviceID != "" {
		account.DeviceID = strings.TrimSpace(in.DeviceID)
	}
	if in.DeviceModel != "" {
		account.DeviceModel = in.DeviceModel
	}
	if in.SystemVersion != "" {
		account.SystemVersion = in.SystemVersion
	}
	if in.Platform != "" {
		account.Platform = normalizePlatform(in.Platform)
	}
	if in.WisDeviceID != "" {
		account.WisDeviceID = strings.TrimSpace(in.WisDeviceID)
	}
	if in.UserAgent != "" {
		account.UserAgent = strings.TrimSpace(in.UserAgent)
	}
	if in.AppVersion != "" {
		account.AppVersion = strings.TrimSpace(in.AppVersion)
	}
	if in.BuildID != "" {
		account.BuildID = in.BuildID
	}
	if in.Address != "" {
		account.Address = in.Address
	}
	if in.Lon != 0 || in.Lat != 0 {
		account.Lon, account.Lat = in.Lon, in.Lat
	}
	if in.Abnormal != "" {
		account.Abnormal = in.Abnormal
	}
	if err := s.db.UpdateAccount(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// SavePhoto 保存上传的照片到账号目录，返回相对路径。
func (s *Service) SavePhoto(userID, accountID int64, filename string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("照片内容为空")
	}
	if len(data) > 8<<20 {
		return "", errors.New("照片不能超过 8MB")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return "", errors.New("仅支持 jpg/png/webp 格式")
	}
	dir := filepath.Join(s.cfg.PhotosDir(), fmt.Sprintf("%d_%d", userID, accountID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(fmt.Sprintf("%d_%d", userID, accountID), name)), nil
}

// PhotoAbsPath 把相对路径转成绝对路径。
func (s *Service) PhotoAbsPath(rel string) string {
	if rel == "" {
		return ""
	}
	return filepath.Join(s.cfg.PhotosDir(), filepath.FromSlash(rel))
}

// clientFor 用账号保存的 cookie 构造协议客户端。
func (s *Service) clientFor(a *store.Account) *cpdaily.Client {
	return cpdaily.NewClient(s.opts, decodeCookies(a.Cookies))
}

// deviceProfile 从账号生成设备信息。
func deviceProfile(a *store.Account, lon, lat float64) cpdaily.DeviceProfile {
	platform := normalizePlatform(a.Platform)
	return cpdaily.DeviceProfile{
		DeviceID:      a.DeviceID,
		DeviceModel:   a.DeviceModel,
		SystemVersion: a.SystemVersion,
		Platform:      platform,
		WisDeviceID:   a.WisDeviceID,
		RawUserAgent:  a.UserAgent,
		BuildID:       a.BuildID,
		SystemName:    map[string]string{"ios": "iOS"}[platform],
		AppVersion:    firstNonEmpty(a.AppVersion, cpdaily.AppVersion),
		UserID:        a.Username,
		Lon:           lon,
		Lat:           lat,
	}
}

// normalizePlatform 统一平台标识，仅支持 android / ios。
func normalizePlatform(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ios", "iphone", "ipad":
		return "ios"
	default:
		return "android"
	}
}

func encodeCookies(cookies map[string]string) string {
	buf, _ := json.Marshal(cookies)
	return string(buf)
}

func decodeCookies(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// inheritableCookies 是跨登录方式仍然值得保留的键。
//
// CASTGC 是 CAS 的 TGT，可反复换 service ticket，是“静默续期”的唯一凭据；
// CONVERSATION 是同一 CAS 会话的标识。两者与用哪种因子登录无关。
// MOD_AUTH_CAS / sessionToken / tenantId 属于本次登录的结果，不能让旧值盖过新值。
var inheritableCookies = []string{"CASTGC", "CONVERSATION"}

// mergeCookies 用新会话覆盖旧 cookie，但保留可继承的 CAS 票据。
//
// 短信登录只换回 MOD_AUTH_CAS（不带 CASTGC）。如果整体替换旧 cookie，
// 一个原本用账密登录、持有 TGT 的账号，改走短信重登后反而丢掉了可续期的票据——
// 登录方式切换不该以“降级成短命会话”为代价。
func mergeCookies(oldRaw string, fresh map[string]string) map[string]string {
	out := make(map[string]string, len(fresh)+2)
	for k, v := range fresh {
		if strings.TrimSpace(v) == "" {
			continue
		}
		out[k] = v
	}
	old := decodeCookies(oldRaw)
	for _, k := range inheritableCookies {
		if out[k] == "" && old[k] != "" {
			out[k] = old[k]
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
