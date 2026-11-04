package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 是服务端运行配置。
type Config struct {
	Addr        string // 监听地址，如 :8090
	DataDir     string // 数据目录（db、照片、密钥）
	SecretKey   []byte // 账号密码加密密钥（AES-256-GCM）
	AutoMigrate bool
	OpenAPI     bool   // 是否开放 /api 给匿名（默认关闭，仅登录后可用）
	AdminUser   string // 首次启动创建的管理员账号（空则不创建）
	AdminPass   string
	// QuotaEnabled 开启额度/卡密体系。
	// 面向「托管运营」场景（多用户 + 卡密充值）；个人自部署保持默认关闭，
	// 关闭时执行不扣额度、兑换接口不可用、前端隐藏额度相关 UI。
	QuotaEnabled bool
	// RegisterOpen 控制注册接口是否对外开放。
	//
	// 默认 false：仅当库中还没有任何用户（首次初始化）时允许注册，
	// 之后注册口自动关闭，避免公网实例被陌生人占用。
	// 需要多人自行注册的场景显式设置 QINGJIN_REGISTER_OPEN=true。
	RegisterOpen bool
	// TLSInsecureHosts 是允许跳过证书校验的主机后缀白名单。
	//
	// 默认为空，即对学校系统做完整证书校验。个别学校证书链不完整时，
	// 用 QINGJIN_TLS_INSECURE_HOSTS 精确列出对应域名（逗号分隔），
	// 而不是像早期版本那样全局关闭校验。
	TLSInsecureHosts []string

	// SingleUser 单用户模式：打开即用，无需注册/登录。
	//
	// 面向「本地双击 exe 就能用」的个人场景：所有接口自动落到唯一的
	// 本地用户（首次启动自动创建），前端跳过登录页。
	//
	// 默认监听 :8090（全部网卡）：扫码授权需要手机能访问到本机，
	// 二维码才能用局域网地址生成（同 qrcp/LocalSend 等局域网工具的
	// 惯例）。该模式下 API 无鉴权，仅建议家庭网络使用；启动时会打印
	// 警告。只允许本机访问可显式设置 QINGJIN_ADDR=127.0.0.1:8090。
	// 服务器多人使用时设 QINGJIN_MULTI_USER=true 恢复账号体系。
	//
	// 注意零值 = false（多用户）：直接构造 Config 的单测与既有部署
	// 保持原有行为；只有经 config.Load() 读取环境变量时才会默认开启。
	SingleUser bool
}

// Load 从环境变量读取配置，未设置的项使用默认值。
//
// 支持的环境变量：
//
//	QINGJIN_ADDR            监听地址（默认 :8090；单用户模式默认 127.0.0.1:8090）
//	QINGJIN_DATA_DIR        数据目录（默认 ./data）
//	QINGJIN_SECRET          32 字节密钥（base64），不填则自动生成到 <data>/secret.key
//	QINGJIN_ADMIN_USER      管理员用户名（默认空，即首次访问时注册）
//	QINGJIN_ADMIN_PASS      管理员密码
//	QINGJIN_MULTI_USER      多用户/账号体系开关（默认 false=单用户直开；服务器多人用设为 true）
//	QINGJIN_QUOTA_ENABLED   额度/卡密体系开关（默认 false；托管运营场景设为 true）
//	QINGJIN_REGISTER_OPEN   是否允许自助注册（默认 false：仅首次初始化可注册）
//	QINGJIN_TLS_INSECURE_HOSTS  允许跳过证书校验的主机后缀，逗号分隔（默认空=严格校验）
func Load() (*Config, error) {
	multiUser := envBool("QINGJIN_MULTI_USER", false)

	// 单用户模式默认监听全部网卡：扫码授权要求手机能访问到本机。
	// 无鉴权风险见 SingleUser 字段注释；只允许本机可显式设 127.0.0.1:8090。
	addr := envOr("QINGJIN_ADDR", ":8090")
	dataDir := envOr("QINGJIN_DATA_DIR", "./data")

	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败：%w", err)
	}

	key, err := loadSecretKey(absDir)
	if err != nil {
		return nil, err
	}

	return &Config{
		Addr:             addr,
		DataDir:          absDir,
		SecretKey:        key,
		SingleUser:       !multiUser,
		AdminUser:        strings.TrimSpace(os.Getenv("QINGJIN_ADMIN_USER")),
		AdminPass:        os.Getenv("QINGJIN_ADMIN_PASS"),
		QuotaEnabled:     envBool("QINGJIN_QUOTA_ENABLED", false),
		RegisterOpen:     envBool("QINGJIN_REGISTER_OPEN", false),
		TLSInsecureHosts: splitList(os.Getenv("QINGJIN_TLS_INSECURE_HOSTS")),
	}, nil
}

// splitList 解析逗号分隔的配置项，去空白、去空项。
func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// envBool 读取布尔型环境变量（接受 1/true/yes/on，大小写不敏感）。
func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off", "":
		return fallback
	}
	return fallback
}

// loadSecretKey 读取或生成持久化密钥。
func loadSecretKey(dir string) ([]byte, error) {
	if raw := strings.TrimSpace(os.Getenv("QINGJIN_SECRET")); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil || len(key) != 32 {
			return nil, errors.New("QINGJIN_SECRET 必须是 32 字节的 base64 字符串")
		}
		return key, nil
	}

	path := filepath.Join(dir, "secret.key")
	if data, err := os.ReadFile(path); err == nil {
		key := strings.TrimSpace(string(data))
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// EnvInt 读取整数型环境变量。
func EnvInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// PhotosDir 返回照片目录。
func (c *Config) PhotosDir() string { return filepath.Join(c.DataDir, "photos") }

// DBPath 返回数据库文件路径。
func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "qingjin.db") }
