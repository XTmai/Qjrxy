package cpdaily

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Tenant 是学校在今日校园的租户信息。
type Tenant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	JoinType string `json:"joinType"`
	Host     string `json:"host"`
	IDSURL   string `json:"idsUrl"`
	AmpURL   string `json:"ampUrl"`
}

// School 是可搜索的学校条目。
type School struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ClientOptions 控制 HTTP 行为。
type ClientOptions struct {
	Timeout    time.Duration
	Insecure   bool
	ProxyURL   string
	UserAgent  string
	HTTPClient *http.Client
}

func defaultHTTPClient(opts ClientOptions) *http.Client {
	if opts.HTTPClient != nil {
		return opts.HTTPClient
	}
	jar, _ := cookiejar.New(nil)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = insecureTLSConfig(opts.Insecure)
	if opts.ProxyURL != "" {
		if u, err := url.Parse(opts.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Jar: jar, Timeout: timeout, Transport: transport}
}

// Client 封装一个账号的会话。
type Client struct {
	http    *http.Client
	opts    ClientOptions
	cookies map[string]string
}

// NewClient 创建客户端；cookies 为可选的 cookie 键值对（用于导入登录态）。
func NewClient(opts ClientOptions, cookies map[string]string) *Client {
	c := &Client{http: defaultHTTPClient(opts), opts: opts, cookies: map[string]string{}}
	for k, v := range cookies {
		if strings.TrimSpace(k) != "" {
			c.cookies[k] = v
		}
	}
	return c
}

// HTTP 返回底层客户端，便于上层复用（例如上传文件）。
func (c *Client) HTTP() *http.Client { return c.http }

// Cookies 返回当前 cookie 快照。
func (c *Client) Cookies() map[string]string {
	out := make(map[string]string, len(c.cookies))
	for k, v := range c.cookies {
		out[k] = v
	}
	return out
}

// SetCookies 覆盖 cookie（登录后或导入时使用）。
func (c *Client) SetCookies(cookies map[string]string) {
	c.cookies = map[string]string{}
	for k, v := range cookies {
		c.cookies[k] = v
	}
}

// ImportCookieString 解析浏览器复制的 "a=1; b=2" 形式 Cookie。
func ImportCookieString(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return out
}

// CookieString 把 cookie 拼回请求头形式。
func (c *Client) CookieString() string {
	if len(c.cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.cookies))
	for k, v := range c.cookies {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) hasSession() bool {
	return c.cookies["MOD_AUTH_CAS"] != "" || c.cookies["sessionToken"] != ""
}

// absorb 把响应 Set-Cookie 合并进会话。
func (c *Client) absorb(resp *http.Response) {
	for _, ck := range resp.Cookies() {
		if ck.Name == "" {
			continue
		}
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
			continue
		}
		c.cookies[ck.Name] = ck.Value
	}
}

func (c *Client) do(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// 调用方显式给了 Cookie（如 iOS 原生客户端需要手工拼 MOD_AUTH_CAS）时不覆盖，
	// 否则会把服务端要求的会话票冲掉，导致返回登录页 HTML。
	if req.Header.Get("Cookie") == "" {
		if cookie := c.CookieString(); cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		ua := c.opts.UserAgent
		if ua == "" {
			ua = "Mozilla/5.0 (Linux; Android 14; " + DefaultBuildID +
				" Build/AP2A.240705.005; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 " +
				"Chrome/120.0.6099.230 Mobile Safari/537.36 okhttp/4.12.0 cpdaily/" + AppVersion +
				" wisedu/" + AppVersion
		}
		req.Header.Set("User-Agent", ua)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	c.absorb(resp)
	return resp, nil
}

// apiResponse 是今日校园通用返回结构。
type apiResponse struct {
	Code    flexString      `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"datas"`
	Raw     json.RawMessage `json:"-"`
}

// flexString 兼容 code 既可能是字符串也可能是数字的情况。
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	*f = flexString(s)
	return nil
}

func (c *Client) postJSON(ctx context.Context, rawURL string, headers map[string]string, payload any) (*apiResponse, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(buf))
	} else {
		body = strings.NewReader("{}")
	}
	h := map[string]string{}
	for k, v := range baseHeaders {
		h[k] = v
	}
	for k, v := range headers {
		h[k] = v
	}
	resp, err := c.do(ctx, http.MethodPost, rawURL, h, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("接口返回 %d：%s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("响应不是有效 JSON：%s", truncate(string(raw), 200))
	}
	out.Raw = raw
	if !isSuccessCode(string(out.Code)) {
		msg := out.Message
		if msg == "" {
			msg = truncate(string(raw), 200)
		}
		return &out, &APIError{Code: string(out.Code), Message: msg}
	}
	return &out, nil
}

// isSuccessCode 判断业务返回码是否表示成功。
//
// 今日校园不同子系统的成功码并不统一：签到/查寝业务用 "0"，
// IAP 统一认证（/iap/*）用 "200"。空 code 视为成功（部分接口不返回 code）。
func isSuccessCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "0", "200", "ok", "success", "true":
		return true
	}
	return false
}

// APIError 表示业务失败（code 非成功码）。
type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s（code=%s）", e.Message, e.Code)
}

// IsAuthExpired 判断是否为登录失效。
func IsAuthExpired(err error) bool {
	if err == nil {
		return false
	}
	var ae *APIError
	if errors.As(err, &ae) {
		if ae.Code == "401" || ae.Code == "403" {
			return true
		}
		msg := ae.Message
		for _, kw := range []string{"登录", "未授权", "会话", "超时", "token", "重新登录"} {
			if strings.Contains(msg, kw) {
				return true
			}
		}
	}
	return false
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// doNoRedirect 与 do 相同，但不自动跟随重定向，用于需要读取 Location 的场景。
func (c *Client) doNoRedirect(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Cookie") == "" {
		if cookie := c.CookieString(); cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}

	// 复用同一份 Transport / Jar，但临时禁用跟随
	noFollow := &http.Client{
		Transport: c.http.Transport,
		Jar:       c.http.Jar,
		Timeout:   c.http.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return nil, err
	}
	c.absorb(resp)
	return resp, nil
}
