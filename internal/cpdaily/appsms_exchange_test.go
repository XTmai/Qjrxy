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

// TestExchangeHopPolicy 覆盖换票链路逐跳策略：
// 同站带 CAS 票据；官方网关放行但只带账号体系 sessionToken（不带 CAS 票据）；
// 伪装域名 / 他校回指 / 非授权路径 / authTransit 中转页一律拦下。
func TestExchangeHopPolicy(t *testing.T) {
	const host = "fyvtc.campusphere.net"
	good := gatewayURL("https://" + host + "/iap/oauthCallback")
	cases := []struct {
		name   string
		target string
		want   exchangeHop
	}{
		{"本校换票入口", "https://" + host + "/iap/login?service=x", hopSameSite},
		{"本校回调", "https://" + host + "/iap/oauthCallback?code=x", hopSameSite},
		{"官方网关回指本校", good, hopOAuthGateway},
		{"官方网关子域", strings.Replace(good, "api.campushoy.com", "sso.api.campushoy.com", 1), hopOAuthGateway},
		{"相对 redirect_uri 按本校补全", gatewayURL("/iap/oauthCallback"), hopOAuthGateway},
		{"authTransit 中转页不放行", "https://api.campushoy.com/h5/authTransit/index.html?version=1", hopDeny},
		{"伪装域名不放行", strings.Replace(good, "api.campushoy.com", "api.campushoy.com.attacker.com", 1), hopDeny},
		{"网关非授权路径不放行", "https://api.campushoy.com/steal?redirect_uri=" + url.QueryEscape("https://"+host+"/iap/oauthCallback"), hopDeny},
		{"redirect_uri 指向他校不放行", gatewayURL("https://evil.campusphere.net/iap/oauthCallback"), hopDeny},
		{"redirect_uri 指向本校非回调路径不放行", gatewayURL("https://" + host + "/portal/login"), hopDeny},
		{"缺少 redirect_uri 不放行", "https://api.campushoy.com/connect/oauth2/authorize", hopDeny},
		{"普通第三方不放行", "https://third.example.com/steal", hopDeny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exchangeHopPolicy(tc.target, host); got != tc.want {
				t.Fatalf("got hop=%v, want hop=%v", got, tc.want)
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
		if got := exchangeHopPolicy(gatewayURL("https://"+h+"/iap/oauthCallback"), h); got != hopOAuthGateway {
			t.Errorf("%s: 官方网关应放行（只带 sessionToken，不带 CAS 票据），got hop=%v", h, got)
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

// TestExchangeTGCViaOfficialGateway 模拟完整换票链路（与线上实测的四跳一致），验证：
//  1. 经过官方 OAuth 网关的那一跳：不带 TGC 头、不带 CASTGC/AUTHTGC，只带 sessionToken；
//  2. 网关看到 sessionToken 后直接 302 回本校回调（而不是丢到 authTransit）；
//  3. 回到本校回调后能拿到新的 MOD_AUTH_CAS，换票成功。
func TestExchangeTGCViaOfficialGateway(t *testing.T) {
	var gatewaySeen atomic.Bool
	var gatewayLeaked atomic.Bool
	var gatewayHasSession atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/iap/login":
			loc := gatewayURL("https://fyvtc.campusphere.net/iap/oauthCallback")
			http.Redirect(w, r, loc, http.StatusFound)
		case "/connect/oauth2/authorize":
			gatewaySeen.Store(true)
			ck := r.Header.Get("Cookie")
			// 学校 CAS 票据不得出现在跨域那一跳
			if r.Header.Get("TGC") != "" || strings.Contains(ck, "CASTGC=") || strings.Contains(ck, "AUTHTGC=") {
				gatewayLeaked.Store(true)
			}
			// 必须带账号体系 sessionToken，否则网关会把我们丢到 authTransit
			if strings.Contains(ck, "sessionToken=probe-session") {
				gatewayHasSession.Store(true)
				http.Redirect(w, r, r.URL.Query().Get("redirect_uri")+"?code=fake-code", http.StatusFound)
				return
			}
			http.Redirect(w, r, "https://api.campushoy.com/h5/authTransit/index.html?version=1", http.StatusFound)
		case "/iap/oauthCallback":
			http.SetCookie(w, &http.Cookie{Name: "CASTGC", Value: "TGT-new", Path: "/"})
			http.Redirect(w, r, "https://fyvtc.campusphere.net/portal/login?ticket=ST-iap%3A1", http.StatusFound)
		case "/portal/login":
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

	client := NewClient(ClientOptions{}, map[string]string{"sessionToken": "probe-session"})
	if err := ExchangeTGC(context.Background(), client, "fyvtc.campusphere.net", "TGT-test", DeviceProfile{DeviceID: "dev"}, "fyvtc"); err != nil {
		t.Fatalf("换票应成功：%v", err)
	}
	if !gatewaySeen.Load() {
		t.Fatal("链路没有经过官方 OAuth 网关")
	}
	if gatewayLeaked.Load() {
		t.Fatal("官方网关那一跳带了学校 CAS 票据，存在外泄风险")
	}
	if !gatewayHasSession.Load() {
		t.Fatal("官方网关那一跳没带 sessionToken，会被丢到 authTransit 导致换票失败")
	}
	if got := client.Cookies()["MOD_AUTH_CAS"]; got != "ST-new-from-oauth" {
		t.Fatalf("未拿到新业务票：%q", got)
	}
}

// TestExchangeTGCWithoutSessionGoesAuthTransit 校验：没有 sessionToken 时网关会
// 丢到 authTransit，此时必须给出「需要重新登录」的明确提示，而不是含糊的
// 「跳出了学校站点」。
func TestExchangeTGCWithoutSessionGoesAuthTransit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/iap/login" {
			http.Redirect(w, r, gatewayURL("https://fyvtc.campusphere.net/iap/oauthCallback"), http.StatusFound)
			return
		}
		http.Redirect(w, r, "https://api.campushoy.com/h5/authTransit/index.html?version=1", http.StatusFound)
	}))
	defer srv.Close()

	target, _ := url.Parse(srv.URL)
	original := exchangeClientFactory
	exchangeClientFactory = func() *http.Client {
		return &http.Client{
			Timeout:   20 * time.Second,
			Transport: rewriteRoundTripper{target: target, base: srv.Client().Transport},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	defer func() { exchangeClientFactory = original }()

	client := NewClient(ClientOptions{}, nil)
	err := ExchangeTGC(context.Background(), client, "fyvtc.campusphere.net", "TGT-test", DeviceProfile{DeviceID: "dev"}, "fyvtc")
	if err == nil {
		t.Fatal("没有 sessionToken 时不应判定换票成功")
	}
	if !strings.Contains(err.Error(), "重新登录") {
		t.Fatalf("错误信息应提示重新登录，实际：%v", err)
	}
}
