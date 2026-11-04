package cpdaily

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// TLS 策略：默认对学校系统做完整的证书链校验。
//
// 早期实现为了兼容"证书链不完整"的学校，把 InsecureSkipVerify 全局打开，
// 这会让服务器与学校之间的链路可被中间人劫持——而这条路传的是学号与密码。
// 现在改为默认严格校验，只有显式列入白名单的主机才跳过校验，
// 由部署者按学校具体情况逐个放开（环境变量 QINGJIN_TLS_INSECURE_HOSTS）。
var (
	tlsMu            sync.RWMutex
	insecureSuffixes []string
)

// ConfigureTLSInsecureHosts 设置允许跳过证书校验的主机后缀（如 "campusphere.net"）。
//
// 传入空列表即恢复为全部严格校验。后缀匹配同时覆盖子域：
// "campusphere.net" 会匹配 "kjxyctgu.campusphere.net"。
func ConfigureTLSInsecureHosts(hosts []string) {
	cleaned := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimPrefix(h, ".")
		if h != "" {
			cleaned = append(cleaned, h)
		}
	}
	tlsMu.Lock()
	insecureSuffixes = cleaned
	tlsMu.Unlock()
}

// TLSInsecureHosts 返回当前白名单副本（供诊断输出）。
func TLSInsecureHosts() []string {
	tlsMu.RLock()
	defer tlsMu.RUnlock()
	return append([]string(nil), insecureSuffixes...)
}

// hostAllowlisted 判断主机名是否落在白名单内（含子域）。
func hostAllowlisted(serverName string, allow []string) bool {
	name := strings.ToLower(strings.TrimSuffix(serverName, "."))
	if name == "" {
		return false
	}
	for _, suffix := range allow {
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}

// tlsConfigFor 按白名单构造 TLS 配置。
//
// 白名单为空时走 Go 默认校验；非空时关闭默认校验，改由 VerifyConnection
// 按实际连接的主机名决定——白名单内跳过，其余仍然做完整的链校验。
func tlsConfigFor(allow []string) *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(allow) == 0 {
		return cfg
	}
	cfg.InsecureSkipVerify = true
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if hostAllowlisted(cs.ServerName, allow) {
			return nil
		}
		return verifyPeerChain(cs)
	}
	return cfg
}

// verifyPeerChain 对单个连接做系统根证书校验。
func verifyPeerChain(cs tls.ConnectionState) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("服务器未提供证书")
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	intermediates := x509.NewCertPool()
	for _, cert := range cs.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	_, err = cs.PeerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       cs.ServerName,
		Roots:         roots,
		Intermediates: intermediates,
	})
	if err != nil {
		return fmt.Errorf("证书校验失败（%s）：%w", cs.ServerName, err)
	}
	return nil
}
