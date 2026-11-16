package cpdaily

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func gatewayURL(redirect string) string {
	return "https://api.campushoy.com/connect/oauth2/authorize?response_type=code" +
		"&client_id=16638349862293443&redirect_uri=" + url.QueryEscape(redirect) + "&state=abc"
}

// TestExchangeHopPolicy 覆盖换票链路逐跳策略：同站带票、官方网关放行但不带票、
// 伪装域名 / 他校回指 / 非授权路径仍一律拦下。
func TestExchangeHopPolicy(t *testing.T) {
	const host = "fyvtc.campusphere.net"
	good := gatewayURL("https://" + host + "/iap/oauthCallback")
	cases := []struct {
		name   string
		target string
		allow  bool
		creds  bool
	}{
		{"本校换票入口", "https://" + host + "/iap/login?service=x", true, true},
		{"本校回调", "https://" + host + "/iap/oauthCallback?code=x", true, true},
		{"官方网关回指本校", good, true, false},
		{"官方网关子域", strings.Replace(good, "api.campushoy.com", "sso.api.campushoy.com", 1), true, false},
		{"相对 redirect_uri 按本校补全", gatewayURL("/iap/oauthCallback"), true, false},
		{"伪装域名不放行", strings.Replace(good, "api.campushoy.com", "api.campushoy.com.attacker.com", 1), false, false},
		{"网关非授权路径不放行", "https://api.campushoy.com/steal?redirect_uri=" + url.QueryEscape("https://"+host+"/iap/oauthCallback"), false, false},
		{"redirect_uri 指向他校不放行", gatewayURL("https://evil.campusphere.net/iap/oauthCallback"), false, false},
		{"redirect_uri 指向本校非回调路径不放行", gatewayURL("https://" + host + "/portal/login"), false, false},
		{"缺少 redirect_uri 不放行", "https://api.campushoy.com/connect/oauth2/authorize", false, false},
		{"普通第三方不放行", "https://third.example.com/steal", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allow, creds := exchangeHopPolicy(tc.target, host)
			if allow != tc.allow || creds != tc.creds {
				t.Fatalf("got allow=%v creds=%v, want allow=%v creds=%v", allow, creds, tc.allow, tc.creds)
			}
		})
	}
}

// TestExchangeHopPolicyCoversNotCloudHosts 用日志里实际被拦的 6 所学校 +
// 安徽师大做回归：判定不枚举学校名单，凡走同一套官方 OAuth 协议的都自动适用。
func TestExchangeHopPolicyCoversNotCloudHosts(t *testing.T) {
	hosts := []string{
		"ysu.campusphere.net", "cmoc.campusphere.net", "fyvtc.campusphere.net",
		"cqwu.campusphere.net", "cumt.campusphere.net", "xhjw.campusphere.net",
		"ahnu.campusphere.net",
	}
	for _, h := range hosts {
		allow, creds := exchangeHopPolicy(gatewayURL("https://"+h+"/iap/oauthCallback"), h)
		if !allow || creds {
			t.Errorf("%s: 官方网关应放行且不带票据，got allow=%v creds=%v", h, allow, creds)
		}
	}
}

// rewriteRoundTripper 把请求改写到本地测试服务，但保留原始 URL 的 host，
// 这样 exchangeHopPolicy 仍按真实域名判断。
type rewriteRoundTripper struct {
	target *url.URL
	base   http.RoundTripper
}

func (rt rewriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = rt.target.Scheme
	clone.URL.Host = rt.target.Host
	clone.Host = rt.target.Host
	return rt.base.RoundTrip(clone)
}

// TestExchangeTGCViaOfficialGateway 模拟完整换票链路，验证：
//  1. 经过官方 OAuth 网关的那一跳不带 TGC 头 / 票据 Cookie；
//  2. 回到本校回调后能拿到新的 MOD_AUTH_CAS，换票成功。
func TestExchangeTGCViaOfficialGateway(t *testing.T) {
	var gatewaySeen atomic.Bool
	var gatewayLeaked atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iap/login":
			loc := gatewayURL("https://fyvtc.campusphere.net/iap/oauthCallback")
			http.Redirect(w, r, loc, http.StatusFound)
		case "/connect/oauth2/authorize":
			gatewaySeen.Store(true)
			if r.Header.Get("TGC") != "" || r.Header.Get("Cookie") != "" {
				gatewayLeaked.Store(true)
			}
			http.Redirect(w, r, r.URL.Query().Get("redirect_uri"), http.StatusFound)
		case "/iap/oauthCallback":
			http.SetCookie(w, &http.Cookie{Name: "MOD_AUTH_CAS", Value: "ST-new-from-oauth", Path: "/"})
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	target, _ := url.Parse(srv.URL)
	original := exchangeClientFactory
	exchangeClientFactory = func() *http.Client {
		return &http.Client{
			Timeout: 20 * time.Second,
			Transport: rewriteRoundTripper{
				target: target,
				base:   srv.Client().Transport,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	defer func() { exchangeClientFactory = original }()

	client := NewClient(ClientOptions{}, nil)
	if err := ExchangeTGC(context.Background(), client, "fyvtc.campusphere.net", "TGT-test", DeviceProfile{DeviceID: "dev"}, "fyvtc"); err != nil {
		t.Fatalf("换票应成功：%v", err)
	}
	if !gatewaySeen.Load() {
		t.Fatal("链路没有经过官方 OAuth 网关")
	}
	if gatewayLeaked.Load() {
		t.Fatal("官方网关那一跳带了票据，存在外泄风险")
	}
	if got := client.Cookies()["MOD_AUTH_CAS"]; got != "ST-new-from-oauth" {
		t.Fatalf("未拿到新业务票：%q", got)
	}
}
