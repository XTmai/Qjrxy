package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"qjrxy/internal/captcha"
	"qjrxy/internal/config"
	"qjrxy/internal/cpdaily"
	"qjrxy/internal/diag"
	"qjrxy/internal/events"
	"qjrxy/internal/secure"
	"qjrxy/internal/store"
	"qjrxy/internal/vision"
)

// Service 聚合业务逻辑，供 HTTP 层与调度器共用。
type Service struct {
	cfg  *config.Config
	db   *store.DB
	opts cpdaily.ClientOptions
	// hub 把数据变更推给在线页面；没有订阅者时发布是空操作。
	hub *events.Hub
	// captchas 保存「等用户手点」的签到验证码会话，进程内有效。
	captchas *captcha.Broker

	mu    sync.RWMutex
	state map[string]*cpdaily.LoginState // loginTicket -> 登录中间态

	// 学校入口解析缓存：批量导入时同一学校会重复出现，避免每次都打远端。
	tenantMu    sync.Mutex
	tenantCache map[string]*cpdaily.Tenant

	// photoMu 串行化「容量检查 + 落盘」，避免并发上传同时通过上限检查。
	photoMu sync.Mutex
}

// New 创建服务。
func New(cfg *config.Config, db *store.DB) *Service {
	photos := cfg.PhotosDir()
	_ = os.MkdirAll(photos, 0o755)
	// 证书校验策略：默认对学校系统做完整校验，只有配置里显式列出的主机才跳过。
	// 这条链路承载学号与密码，不能再按早期版本全局关闭校验。
	cpdaily.ConfigureTLSInsecureHosts(cfg.TLSInsecureHosts)
	svc := &Service{
		cfg:      cfg,
		db:       db,
		hub:      events.New(),
		captchas: captcha.NewBroker(captcha.DefaultTTL),
		state:    map[string]*cpdaily.LoginState{},
		opts: cpdaily.ClientOptions{
			Timeout:       40 * time.Second,
			InsecureHosts: cfg.TLSInsecureHosts,
		},
	}
	// 诊断日志与识别服务：env 为底，settings 表（管理员页改的）覆盖。
	// 两者都支持运行时热更新（admin API 保存后立即生效）。
	svc.ApplyRuntimeSettings()
	return svc
}

// applyRuntimeSettings 按「settings 表 > 环境变量 > 内置默认」的优先级
// 重载诊断日志与识别服务配置。启动与管理员保存后共用。
// ApplyRuntimeSettings 按「settings 表 > 环境变量 > 内置默认」的优先级
// 重载诊断日志与识别服务配置。启动与管理员保存后共用。
func (s *Service) ApplyRuntimeSettings() {
	ctx := context.Background()
	settings, _ := s.db.AllSettings(ctx)

	diagOn := os.Getenv("QINGJIN_DIAG") != "off"
	if v := settings["diag.enabled"]; v != "" {
		diagOn = v != "0"
	}
	keep := 7
	if v := settings["diag.keepDays"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			keep = n
		}
	}
	diag.Configure(s.cfg.DataDir, diagOn, keep)

	key := os.Getenv("DEEPSEEK_API_KEY")
	if enc := settings["vision.apiKey"]; enc != "" {
		if plain, err := secure.Decrypt(s.cfg.SecretKey, enc); err == nil {
			key = plain
		}
	}
	base := os.Getenv("DEEPSEEK_BASE_URL")
	if v := settings["vision.baseURL"]; v != "" {
		base = v
	}
	model := os.Getenv("DEEPSEEK_VISION_MODEL")
	if v := settings["vision.model"]; v != "" {
		model = v
	}
	vision.Configure(key, base, model)

	// 服务器运行模式：settings 覆盖环境变量，直接写回共享的 cfg 指针，
	// 所有读取 cfg 的路径（鉴权、额度、systemInfo）立即看到新值。
	// settings 表一旦写过某个键就永久压过环境变量——这个优先级对部署者
	// 并不显而易见，覆盖发生时必须打到日志里，否则「env 设了多用户却没生效」
	// 会变成一笔查不到的糊涂账。
	if v := settings["server.multiUser"]; v != "" {
		s.cfg.SingleUser = v == "0"
		log.Printf("运行模式由 settings 覆盖环境变量：多用户=%v", !s.cfg.SingleUser)
	}
	if v := settings["server.registerOpen"]; v != "" {
		s.cfg.RegisterOpen = v == "1"
	}
	if v := settings["server.quotaEnabled"]; v != "" {
		s.cfg.QuotaEnabled = v == "1"
	}
	if v := settings["server.tlsInsecureHosts"]; v != "" || dbHas("server.tlsInsecureHosts", settings) {
		hosts := splitHosts(v)
		s.cfg.TLSInsecureHosts = hosts
		cpdaily.ConfigureTLSInsecureHosts(hosts)
		// clientFor 用的是 s.opts 的快照，不同步的话 UI 显示保存成功，
		// 登录/签到的 HTTP 客户端却永远用启动时那份白名单。
		s.opts.InsecureHosts = hosts
	}
}

// splitHosts 解析逗号分隔的白名单，去空白。
func splitHosts(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// dbHas 判断 settings 里是否出现过某个键（区分「空值清空」与「未配置」）。
func dbHas(key string, settings map[string]string) bool {
	_, ok := settings[key]
	return ok
}

// DB 暴露存储层。
func (s *Service) DB() *store.DB { return s.db }

// Config 暴露配置。
func (s *Service) Config() *config.Config { return s.cfg }

// Events 暴露变更广播中心（SSE 与调度器共用）。
func (s *Service) Events() *events.Hub { return s.hub }

// notify 广播数据变更；请求协程与后台调度协程都可以安全调用。
func (s *Service) notify(userID int64, topics ...string) {
	if s.hub != nil {
		s.hub.Publish(userID, topics...)
	}
}

// validCoord 校验经纬度范围。0,0 表示未设置，由调用方决定是否写入。
func validCoord(lon, lat float64) bool {
	return lon >= -180 && lon <= 180 && lat >= -90 && lat <= 90
}

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
	user, err := s.db.CreateUser(ctx, username, nickname, hash, 50)
	if err != nil {
		return nil, err
	}
	// 第一个注册的用户就是管理员：部署者注册完即可用管理员页，
	// 不需要再手动改库。注释承诺了很多年的行为在这里兑现。
	if count, cerr := s.db.CountUsers(ctx); cerr == nil && count == 1 {
		_ = s.db.UpdateUserRole(ctx, user.ID, "admin")
		user.Role = "admin"
	}
	return user, nil
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
	if (in.Lon != 0 || in.Lat != 0) && !validCoord(in.Lon, in.Lat) {
		return nil, false, errors.New("经纬度超出有效范围")
	}
	if err := validateDeviceInput(&in); err != nil {
		return nil, false, err
	}
	tenant, err := s.resolveTenantCached(ctx, in.SchoolName)
	if err != nil {
		return nil, false, err
	}
	// 空密码（NOTCLOUD 扫码登录场景）不加密空串，直接留空：
	// 否则会存一段「空密码」的密文，hasCredentials 会误判成「有密码可自动重登」。
	var passwordEnc string
	if strings.TrimSpace(in.Password) != "" {
		var err error
		passwordEnc, err = secure.Encrypt(s.cfg.SecretKey, in.Password)
		if err != nil {
			return nil, false, err
		}
	}
	username := strings.TrimSpace(in.Username)

	// 先查一次：已绑定则更新密码/设备信息后返回，不报 UNIQUE。
	existing, err := s.db.AccountBySchoolUsername(ctx, userID, tenant.Name, username)
	if err == nil {
		if err := s.rebindAccount(ctx, existing, in, tenant, passwordEnc); err != nil {
			return nil, false, err
		}
		s.notify(userID, events.TopicAccounts)
		return existing, true, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, false, err
	}

	// NOTCLOUD 学校用自建统一身份认证，登录页形态各校不同（滑块、校内短信、
	// 自定义表单），当前无法通用适配。
	//
	// 只在「确实要新建账号」时拦截：上面那条 rebind 分支必须放行——
	// 已有账号（含早先为适配过的学校建的）仍要能改密码/重新绑定，
	// 否则那些账号就被彻底锁死，连自救的机会都没有。
	// NOTCLOUD 学校用自建统一身份认证，登录页形态各校不同（滑块、校内短信、
	// 自定义表单），账密/短信登录无法通用适配。
	//
	// 但这类学校可以走「扫码获取登录态」：借 App 已登录的会话换出业务票据。
	// 所以只在「要建账号」时要求密码留空——有密码说明用户想走账密登录，
	// 那对 NOTCLOUD 学校一定会失败，不如当场说清；留空就建一条只有学校+学号
	// 的记录，扫码补齐会话即可。
	notCloud := !cpdaily.SchoolSupported(tenant.JoinType)
	if notCloud && strings.TrimSpace(in.Password) != "" {
		return nil, false, fmt.Errorf(
			"「%s」使用学校自建统一身份认证，不支持账密/短信登录。"+
				"请把密码留空创建账号，然后在账号卡片上点「扫码登录」获取登录态",
			tenant.Name)
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
		s.notify(userID, events.TopicAccounts)
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	s.notify(userID, events.TopicAccounts)
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
		if !validCoord(in.Lon, in.Lat) {
			return errors.New("经纬度超出有效范围")
		}
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
	if in.DeviceID != "" || in.UserAgent != "" {
		if err := validateDeviceInput(&in); err != nil {
			return nil, err
		}
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
		if !validCoord(in.Lon, in.Lat) {
			return nil, errors.New("经纬度超出有效范围")
		}
		account.Lon, account.Lat = in.Lon, in.Lat
	}
	if in.Abnormal != "" {
		account.Abnormal = in.Abnormal
	}
	if err := s.db.UpdateAccount(ctx, account); err != nil {
		return nil, err
	}
	s.notify(userID, events.TopicAccounts)
	return account, nil
}

// PhotoPoolLimitKey 是管理员页保存的照片池总容量配置项。
const PhotoPoolLimitKey = "photo.poolLimitBytes"

// DefaultPhotoPoolLimitBytes 是照片池总容量默认上限（1GB）。
const DefaultPhotoPoolLimitBytes int64 = 1 << 30

// SavePhoto 保存上传的单张兜底照片到账号目录，返回相对路径。
func (s *Service) SavePhoto(userID, accountID int64, filename string, data []byte) (string, error) {
	dir := filepath.Join(s.cfg.PhotosDir(), fmt.Sprintf("%d_%d", userID, accountID))
	return s.savePhotoFile(dir, filename, data)
}

// SavePoolPhoto 保存照片池中的一张，放在账号目录的 pool/ 子目录下。
//
// 全局容量上限（所有账号的照片池合计，默认 1GB）在这里统一执行：
// 检查与落盘用 photoMu 串行化，避免并发上传同时通过检查。
func (s *Service) SavePoolPhoto(ctx context.Context, userID, accountID int64, filename string, data []byte) (string, error) {
	if _, err := validatePhoto(filename, data); err != nil {
		return "", err
	}
	s.photoMu.Lock()
	defer s.photoMu.Unlock()
	limit := s.PhotoPoolLimit(ctx)
	used, _ := s.PhotoPoolUsage()
	if used+int64(len(data)) > limit {
		return "", fmt.Errorf("照片池总容量已满（已用 %s，上限 %s）", formatStorage(used), formatStorage(limit))
	}
	dir := filepath.Join(s.cfg.PhotosDir(), fmt.Sprintf("%d_%d", userID, accountID), "pool")
	return s.savePhotoFile(dir, filename, data)
}

// PhotoPoolLimit 返回照片池总容量上限；未配置或配置非法时用 1GB。
func (s *Service) PhotoPoolLimit(ctx context.Context) int64 {
	raw, _ := s.db.GetSetting(ctx, PhotoPoolLimitKey)
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err == nil && n > 0 {
		return n
	}
	return DefaultPhotoPoolLimitBytes
}

// PhotoPoolUsage 统计所有账号照片池的磁盘占用（不含单张兜底照片）。
func (s *Service) PhotoPoolUsage() (used int64, count int) {
	root := s.cfg.PhotosDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		// 目录不存在是首次使用的正常状态；其余错误不能静默当 0，
		// 否则容量上限检查会放行超限上传。
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("统计照片池用量失败（按 0 处理，容量检查可能放行超限）：%v", err)
		}
		return 0, 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, entry.Name(), "pool"))
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			if info, err := file.Info(); err == nil {
				used += info.Size()
				count++
			}
		}
	}
	return used, count
}

// validatePhoto 校验大小与扩展名，返回归一化后的扩展名。
func validatePhoto(filename string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("照片内容为空")
	}
	if len(data) > 5<<20 {
		return "", errors.New("照片不能超过 5MB")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return "", errors.New("仅支持 jpg/png/webp 格式")
	}
	return ext, nil
}

// savePhotoFile 是单张照片与照片池共用的落盘逻辑（大小/格式校验）。
func (s *Service) savePhotoFile(dir, filename string, data []byte) (string, error) {
	ext, err := validatePhoto(filename, data)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// 纳秒在部分平台精度不足（Windows 实测约 1ms），两次快速上传可能
	// 生成同名文件并互相覆盖；追加进程内序号保证唯一。
	name := fmt.Sprintf("%d_%d%s", time.Now().UnixNano(), photoSeq.Add(1), ext)
	abs := filepath.Join(dir, name)
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.cfg.PhotosDir(), abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// photoSeq 为照片文件名提供进程内单调序号。
var photoSeq atomic.Uint64

// formatStorage 把字节数格式化成管理员/错误提示里易读的容量。
func formatStorage(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%dKB", (n+1023)/1024)
	}
}

// runPhoto 为一次执行挑选照片：池优先、顺序轮换，池为空/文件失效时回退单张。
//
// 文件丢失的池记录会被顺手删除，避免每次执行都先撞一张坏图。
func (s *Service) runPhoto(ctx context.Context, account *store.Account) (string, bool) {
	for i := 0; i < 100; i++ {
		photo, err := s.db.NextAccountPhoto(ctx, account.UserID, account.ID)
		if err != nil || photo == nil {
			break
		}
		abs := s.PhotoAbsPath(photo.Path)
		if abs != "" && fileExists(abs) {
			return abs, true
		}
		_ = os.Remove(abs)
		_, _ = s.db.DeleteAccountPhoto(ctx, account.UserID, account.ID, photo.ID)
	}
	abs := s.PhotoAbsPath(account.PhotoPath)
	if abs != "" && fileExists(abs) {
		return abs, false
	}
	return "", false
}

// RemoveAccountPhotoDir 删除账号的照片目录（单张 + 照片池）。
func (s *Service) RemoveAccountPhotoDir(userID, accountID int64) {
	_ = os.RemoveAll(filepath.Join(s.cfg.PhotosDir(), fmt.Sprintf("%d_%d", userID, accountID)))
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
	opts := s.opts
	opts.Label = fmt.Sprintf("account:%d", a.ID)
	return cpdaily.NewClient(opts, decodeCookies(a.Cookies))
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

// validateDeviceInput 校验手动填写/导入的设备字段。
//
// 设备 ID 是提交签到时 CpdailyInfo 里的 CpUUID，长度和字符集与 App 采集的
// 一致；UA 原样透传给请求头，过长会让部分网关直接拒绝。
func validateDeviceInput(in *AccountInput) error {
	if id := strings.TrimSpace(in.DeviceID); id != "" {
		if len(id) < 8 || len(id) > 128 {
			return errors.New("设备 ID 长度需在 8-128 之间")
		}
		for _, r := range id {
			if r <= 0x20 || r == 0x7f {
				return errors.New("设备 ID 不能包含空白或控制字符")
			}
		}
		in.DeviceID = id
	}
	if len(in.UserAgent) > 512 {
		return errors.New("User-Agent 过长（最多 512 字符）")
	}
	return nil
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
