// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import "testing"

// TestOAuthGatewayRedirectTrailingSlash 校验 redirect_uri 带尾斜杠时
// 仍被认作官方网关，而不是误判成「跳出了学校站点」。
//
// 回归（2026-11-21 用户报错）：界面出现
// 「换票链路跳出了学校站点（https://api.campushoy.com/connect/oauth2/
//
//	authorize?...），已中止以免票据外泄」
//
// 该地址其实是官方网关的正确地址。严格等值比较路径时，
// "/iap/oauthCallback/" 这种带尾斜杠的写法会落到 hopDeny，
// 导致整条换票链路中止、NOTCLOUD 账号无法续期。
func TestOAuthGatewayRedirectTrailingSlash(t *testing.T) {
	const origin = "foo.campusphere.net"
	cases := []struct {
		name       string
		redirect   string
		wantPolicy exchangeHop
	}{
		{"标准写法", "https://foo.campusphere.net/iap/oauthCallback", hopOAuthGateway},
		{"带尾斜杠", "https://foo.campusphere.net/iap/oauthCallback/", hopOAuthGateway},
		{"带 query", "https://foo.campusphere.net/iap/oauthCallback?a=1", hopOAuthGateway},
		{"大小写不同", "https://FOO.CAMPUSPHERE.NET/iap/oauthCallback", hopOAuthGateway},
		// 安全性不能放松：指向他校仍必须拒绝
		{"指向他校", "https://evil.campusphere.net/iap/oauthCallback", hopDeny},
		{"非 oauthCallback 路径", "https://foo.campusphere.net/portal/login", hopDeny},
		{"缺 redirect_uri", "", hopDeny},
	}
	for _, c := range cases {
		target := "https://api.campushoy.com/connect/oauth2/authorize?response_type=code&client_id=1"
		if c.redirect != "" {
			target += "&redirect_uri=" + urlEncodeForTest(c.redirect)
		}
		if got := exchangeHopPolicy(target, origin); got != c.wantPolicy {
			names := map[exchangeHop]string{hopDeny: "DENY", hopSameSite: "SAME", hopOAuthGateway: "GATEWAY"}
			t.Errorf("%s：期望 %s，实际 %s", c.name, names[c.wantPolicy], names[got])
		}
	}
}

// urlEncodeForTest 只做最小转义，够本测试用。
func urlEncodeForTest(s string) string {
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			out = append(out, ch)
			continue
		}
		const hex = "0123456789ABCDEF"
		out = append(out, '%', hex[ch>>4], hex[ch&0x0f])
	}
	return string(out)
}
