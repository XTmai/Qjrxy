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
}

// Load 从环境变量读取配置，未设置的项使用默认值。
//
// 支持的环境变量：
//
//	QINGJIN_ADDR            监听地址（默认 :8090）
//	QINGJIN_DATA_DIR        数据目录（默认 ./data）
//	QINGJIN_SECRET          32 字节密钥（base64），不填则自动生成到 <data>/secret.key
//	QINGJIN_ADMIN_USER      管理员用户名（默认空，即首次访问时注册）
//	QINGJIN_ADMIN_PASS      管理员密码
//	QINGJIN_QUOTA_ENABLED   额度/卡密体系开关（默认 false；托管运营场景设为 true）
func Load() (*Config, error) {
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
		Addr:         addr,
		DataDir:      absDir,
		SecretKey:    key,
		AdminUser:    strings.TrimSpace(os.Getenv("QINGJIN_ADMIN_USER")),
		AdminPass:    os.Getenv("QINGJIN_ADMIN_PASS"),
		QuotaEnabled: envBool("QINGJIN_QUOTA_ENABLED", false),
	}, nil
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
