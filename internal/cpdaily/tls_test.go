// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import (
	"crypto/tls"
	"testing"
)

// TestHostAllowlisted 校验白名单匹配不会误放行相似域名。
func TestHostAllowlisted(t *testing.T) {
	allow := []string{"campusphere.net", "cpdaily.example.edu.cn"}
	cases := map[string]bool{
		"campusphere.net":              true,
		"kjxyctgu.campusphere.net":     true,
		"cpdaily.example.edu.cn":       true,
		"campusphere.net.attacker.com": false,
		"notcampusphere.net":           false,
		"other.edu.cn":                 false,
		"":                             false,
	}
	for host, want := range cases {
		if got := hostAllowlisted(host, allow); got != want {
			t.Errorf("hostAllowlisted(%q) = %v，期望 %v", host, got, want)
		}
	}
}

// TestTLSConfigDefaultIsStrict 校验默认（无白名单）时走完整证书校验。
//
// 这是本次修复的核心：早期实现全局 InsecureSkipVerify=true，
// 服务器与学校之间的链路可被中间人劫持，而这条路上传的是学号与密码。
func TestTLSConfigDefaultIsStrict(t *testing.T) {
	cfg := tlsConfigFor(nil)
	if cfg.InsecureSkipVerify {
		t.Fatal("白名单为空时不得关闭证书校验")
	}
	if cfg.VerifyConnection != nil {
		t.Fatal("白名单为空时应使用 Go 默认校验链")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("最低 TLS 版本应为 1.2，得到 %x", cfg.MinVersion)
	}
}

// TestTLSConfigAllowlistStillVerifiesOthers 校验白名单外的host仍然被校验。
func TestTLSConfigAllowlistStillVerifiesOthers(t *testing.T) {
	cfg := tlsConfigFor([]string{"campusphere.net"})
	if !cfg.InsecureSkipVerify || cfg.VerifyConnection == nil {
		t.Fatal("配置白名单后应由 VerifyConnection 按主机名接管校验")
	}
	if err := cfg.VerifyConnection(tls.ConnectionState{ServerName: "kjxyctgu.campusphere.net"}); err != nil {
		t.Errorf("白名单内主机应放行，得到 %v", err)
	}
	// 没有对端证书却要求校验，必须报错——证明白名单外没有被一起放行。
	if err := cfg.VerifyConnection(tls.ConnectionState{ServerName: "evil.example.com"}); err == nil {
		t.Error("白名单外的主机不应被放行")
	}
}

// TestConfigureTLSInsecureHosts 校验配置解析（大小写、空白、空项）。
func TestConfigureTLSInsecureHosts(t *testing.T) {
	ConfigureTLSInsecureHosts([]string{" .Campusphere.NET ", "", "  ", ".x.edu.cn"})
	t.Cleanup(func() { ConfigureTLSInsecureHosts(nil) })

	got := TLSInsecureHosts()
	want := []string{"campusphere.net", "x.edu.cn"}
	if len(got) != len(want) {
		t.Fatalf("白名单解析结果 %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 项为 %q，期望 %q", i, got[i], want[i])
		}
	}
}
