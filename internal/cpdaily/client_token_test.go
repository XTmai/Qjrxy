package cpdaily

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 有 sessionToken 的账号（短信登录）：所有请求都要带账号体系令牌头，
// 服务端才按 token 做长期认证，而不是只认约 12 小时到期的 CAS 票。
func TestDoInjectsSessionTokenHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"code":"0"}`)
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{}, map[string]string{"sessionToken": "token-abc-123"})
	resp, err := c.do(context.Background(), http.MethodPost, srv.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// 抓包确认：App 的 sessionTokenKey 是明文 UUID，不是加密串
	if got.Get("sessionTokenKey") != "token-abc-123" {
		t.Errorf("sessionTokenKey = %q，期望明文 token", got.Get("sessionTokenKey"))
	}
	if got.Get("SessionToken") != "" {
		t.Errorf("App 不带 SessionToken 头，得到 %q", got.Get("SessionToken"))
	}
}

// iOS 与 App 行为一致：只带 sessionTokenKey，不带 SessionToken。
func TestDoIOSSessionTokenHeader(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"code":"0"}`)
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 cpdaily/9.9.24",
	}, map[string]string{"sessionToken": "token-ios"})
	resp, err := c.do(context.Background(), http.MethodPost, srv.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got.Get("sessionTokenKey") == "" {
		t.Error("iOS 也应带 sessionTokenKey")
	}
	if got.Get("SessionToken") != "" {
		t.Errorf("iOS 不应带 SessionToken，得到 %q", got.Get("SessionToken"))
	}
}

// 账密登录的账号没有 sessionToken：不能凭空造头。
func TestDoWithoutSessionToken(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = io.WriteString(w, `{"code":"0"}`)
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{}, map[string]string{"MOD_AUTH_CAS": "ST-x"})
	resp, err := c.do(context.Background(), http.MethodPost, srv.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got.Get("sessionTokenKey") != "" || got.Get("SessionToken") != "" {
		t.Errorf("无 sessionToken 时不应带令牌头：%v", got)
	}
}
