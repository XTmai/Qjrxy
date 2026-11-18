// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	// JoinType 是学校的接入类型：CLOUD（托管认证）/ NOTCLOUD（学校自建统一身份认证）。
	JoinType string `json:"joinType,omitempty"`
	// Supported 表示本项目当前能否为该学校完成登录。
	//
	// 目前只支持 CLOUD：NOTCLOUD 学校的认证页形态各校不同（滑块、校内短信、
	// 自定义表单），协议复刻覆盖不到，需要逐校适配。
	Supported bool `json:"supported"`
}

// ClientOptions 控制 HTTP 行为。
type ClientOptions struct {
	Timeout time.Duration
	// InsecureHosts 是允许跳过证书校验的主机后缀白名单（见 tls.go）。
	// 为空表示对全部主机做严格校验，这是默认且推荐的行为。
	InsecureHosts []string
	ProxyURL      string
	UserAgent     string
	HTTPClient    *http.Client
	// Label 是诊断日志里这路流量的标识（如 "account:22"），可选。
	Label string
}

func defaultHTTPClient(opts ClientOptions) *http.Client {
	if opts.HTTPClient != nil {
		// 外部注入的客户端也要套诊断，否则这条链路的请求在日志里是空的。
		// 复制一份而不是改原对象：调用方可能共用同一个 *http.Client。
		clone := *opts.HTTPClient
		clone.Transport = WrapTransport(opts.HTTPClient.Transport, opts.Label)
		clone.CheckRedirect = chainRedirectHook(opts.HTTPClient.CheckRedirect)
		return &clone
	}
	jar, _ := cookiejar.New(nil)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Jar:     jar,
		Timeout: timeout,
		// Transport 统一套诊断：client.do 之外那些直接走 c.http.Do 的地方
		// （CAS 密码登录提交、换票等）也就能被记录到。
		Transport:     WrapTransport(newTransport(opts), opts.Label),
		CheckRedirect: chainRedirectHook(nil),
	}
}

// redirectTrace 在一条请求的重定向链上记录每一跳的去向。
//
// 需要它是因为「会话失效 → 302 到登录页 → 302 回业务路径 → 405」这条链
// 的**最终** resp.Request.URL 已经是业务路径，看不出中途经过登录页。
// 只有把每一跳都记下来，才能区分「真的方法错误」和「被踢回登录页」。
type redirectTrace struct {
	mu   sync.Mutex
	hops []string
}

func (t *redirectTrace) add(rawURL string) {
	t.mu.Lock()
	t.hops = append(t.hops, rawURL)
	t.mu.Unlock()
}

// passedLoginPath 报告重定向链里是否出现过登录/认证路径。
func (t *redirectTrace) passedLoginPath() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, hop := range t.hops {
		lower := strings.ToLower(hop)
		for _, marker := range sessionRedirectMarkers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

// sessionRedirectMarkers 是登录/认证路径的特征片段。
var sessionRedirectMarkers = []string{"/iap/", "/authserver", "/portal/login", "/cas/"}

type redirectTraceKey struct{}

// chainRedirectHook 返回一个 CheckRedirect：记录每一跳，并沿用调用方
// 原有的策略（原有策略拒绝时照旧拒绝，默认策略下限制 10 跳）。
func chainRedirectHook(existing func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if t, ok := req.Context().Value(redirectTraceKey{}).(*redirectTrace); ok {
			t.add(req.URL.String())
		}
		if existing != nil {
			return existing(req, via)
		}
		return nil // 交给 net/http 的默认上限（10 跳）
	}
}

// newTransport 按选项造一个裸 Transport（TLS/代理策略）。
// 需要诊断的调用方自己再套 WrapTransport。
func newTransport(opts ClientOptions) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfigFor(opts.InsecureHosts)
	if opts.ProxyURL != "" {
		if u, err := url.Parse(opts.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return transport
}

// Client 封装一个账号的会话。
type Client struct {
	http    *http.Client
	opts    ClientOptions
	cookies map[string]string
	label   string
}

// NewClient 创建客户端；cookies 为可选的 cookie 键值对（用于导入登录态）。
func NewClient(opts ClientOptions, cookies map[string]string) *Client {
	c := &Client{http: defaultHTTPClient(opts), opts: opts, cookies: map[string]string{}, label: opts.Label}
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
	return c.doRequest(ctx, method, rawURL, headers, body, true)
}

// doNoCookie 用于对方不该拿到会话票据的请求。
//
// 典型场景是验证码图片：地址由学校服务端下发，可能指向
// wecloud-captcha.campusphere.net 这类 CDN 域。把 MOD_AUTH_CAS
// 送过去既无必要，也等于把登录票据泄露给第三方主机。
func (c *Client) doNoCookie(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader) (*http.Response, error) {
	return c.doRequest(ctx, method, rawURL, headers, body, false)
}

func (c *Client) doRequest(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader, withCookie bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// 调用方显式给了 Cookie（如 iOS 原生客户端需要手工拼 MOD_AUTH_CAS）时不覆盖，
	// 否则会把服务端要求的会话票冲掉，导致返回登录页 HTML。
	if withCookie && req.Header.Get("Cookie") == "" {
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
	// 账号体系令牌：App 对账号与业务接口都带 sessionTokenKey，值是
	// sessionToken 的**明文**（抓包确认：36 字符 UUID，不是加密串）。
	// 这是账号体系的长期凭据，CAS 票过期后服务端仍能据此认人。
	//
	// withCookie=false 的请求（验证码图片等第三方域）必须跳过：它们
	// 不属于校园接口，不该拿到任何账号凭证。
	if withCookie {
		if token := c.cookies["sessionToken"]; token != "" {
			if req.Header.Get("sessionTokenKey") == "" {
				req.Header.Set("sessionTokenKey", token)
			}
		}
	}
	// 诊断由 Transport 层统一记录（见 diagTransport），这里不再手工埋点，
	// 否则同一次请求会在日志里出现重复的两条。
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	c.absorb(resp)
	resp.Body = decompressBody(resp)
	return resp, nil
}

// decompressBody 透明解压 gzip/deflate 响应。
//
// 请求头里手动设置 Accept-Encoding 时，Go 的 http.Client 不会自动解压
// （透明解压只对它自己加的头生效），而今日校园多数接口要求带这个头，
// 会返回压缩响应，直接读取会得到二进制乱码。
func decompressBody(resp *http.Response) io.ReadCloser {
	if resp.Body == nil {
		return resp.Body
	}
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return resp.Body
		}
		return ReadCloser{Reader: gr, Closer: resp.Body}
	case "deflate":
		return ReadCloser{Reader: flate.NewReader(resp.Body), Closer: resp.Body}
	}
	return resp.Body
}

// ReadCloser 组合不同的 Reader 与 Closer。
type ReadCloser struct {
	io.Reader
	io.Closer
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
	// 记录重定向链：405 判定需要知道中途有没有经过登录页。
	trace := &redirectTrace{}
	ctx = context.WithValue(ctx, redirectTraceKey{}, trace)
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
		// 405 常常不是「请求方法写错」，而是会话失效的副产物：
		// POST 业务接口 → 302 到 /iap/login → 重定向按 302 semantics 改成
		// GET → 落到业务路径上 → Tomcat 回 405。实测 2026-11-17 安徽师范
		// 全天都是这条链，而 405 的 body 是 Tomcat 错误页，一个会话失效
		// 关键词都没有，IsAuthExpired 因此返回 false，自动续期永远不触发，
		// 用户只看到「拉取不到任务」。
		if resp.StatusCode == http.StatusMethodNotAllowed && looksLikeSessionRedirect(resp, raw, trace) {
			return nil, &APIError{Code: "401", Message: "登录状态已失效（业务接口被重定向到登录页），需要重新登录"}
		}
		return nil, &APIError{Code: strconv.Itoa(resp.StatusCode),
			Message: truncate(string(raw), 200)}
	}
	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		// 会话失效时服务端直接返回 IAP 登录页 HTML，而不是 JSON。
		// 这里必须构造 APIError 并带上 401 语义，否则 IsAuthExpired 无法识别，
		// 自动重登录就不会触发，用户只能看到「响应不是有效 JSON」。
		if looksLikeLoginPage(raw) || looksLikeHTMLPage(raw) {
			return nil, &APIError{Code: "401", Message: "登录状态已失效，需要重新登录"}
		}
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

// IsAuthExpired 判断错误是否属于「会话失效，需要重新登录」。
//
// 今日校园不同接口表达会话失效的方式并不统一：有的返回 401/403，
// 有的返回登录页 HTML，有的在业务 message 里带「登录」字样，
// 还有的（实测安徽师范）把业务接口 302 到登录页，再以 405 收场。
// 都要识别，否则自动重登录会在部分接口上静默失效。
func IsAuthExpired(err error) bool {
	if err == nil {
		return false
	}
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.Code {
		// 405 只有在 postJSON 判定为「会话重定向」时才会被改写成 401，
		// 所以这里不会把真正的方法错误误判成登录失效。
		case "401", "403":
			return true
		}
		msg := ae.Message
		for _, kw := range []string{"登录", "未授权", "会话", "超时", "token", "重新登录"} {
			if strings.Contains(msg, kw) {
				return true
			}
		}
	}
	// 兜底：错误链里直接带着登录页特征或会话提示的情况
	msg := err.Error()
	for _, kw := range []string{"登录状态已失效", "登录已失效", "登录失效", "需要重新登录", "登录页", "未授权"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// looksLikeHTMLPage 判断响应体是否是一个 HTML 页面。
//
// 业务接口正常只返回 JSON；返回 HTML 基本只有一种情况：会话失效，
// 被重定向/回退到登录页。因此即使登录页文案变了，也能识别。
func looksLikeHTMLPage(raw []byte) bool {
	s := strings.ToLower(strings.TrimLeft(string(raw), "\ufeff \t\r\n"))
	return strings.HasPrefix(s, "<!doctype") || strings.HasPrefix(s, "<html")
}

func looksLikeLoginPage(raw []byte) bool {
	// 部分学校（实测 fdm.lntu.edu.cn）返回的登录页前面带 1~2 个 UTF-8 BOM。
	// 必须先剥掉 BOM 和空白再判断首字符，否则会被当成「不是有效 JSON」的
	// 普通错误，IsAuthExpired 识别不到，自动续期永远不会触发。
	s := strings.TrimLeft(string(raw), "\ufeff \t\r\n")
	if len(s) == 0 || s[0] != '<' {
		return false
	}
	// 登录页的典型特征（统一身份认证 / 重定向脚本 / 表单登录）
	markers := []string{
		"统一身份认证", "iap/login", "/iap/", "authserver",
		"<title>登录", "casLoginForm", "loginFromId",
	}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
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

// looksLikeSessionRedirect 判断一个 4xx 响应是否其实来自「会话失效后被
// 重定向到登录页」这条链，而不是真正的方法/参数错误。
//
// 判定依据（任一条成立即可）：
//   - 这条请求的重定向链里出现过登录/认证路径（最可靠：实测 405 的最终
//     地址已经是业务路径，只有链上看得出中途去过 /iap/login）；
//   - 响应体带有登录页特征（部分部署把登录页直接 200/405 回来）。
//
// 实测样本（2026-11-17 安徽师范）：POST 业务接口 302 → GET /iap/login
// 302 → GET 业务路径 405，405 的 body 是 Apache Tomcat 8.0.50 错误页。
func looksLikeSessionRedirect(resp *http.Response, raw []byte, trace *redirectTrace) bool {
	if trace != nil && trace.passedLoginPath() {
		return true
	}
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		path := strings.ToLower(resp.Request.URL.Path)
		for _, marker := range sessionRedirectMarkers {
			if strings.Contains(path, marker) {
				return true
			}
		}
	}
	// 405 的 body 本身通常没有登录文案，但部分学校会带一个到登录页的
	// 跳转提示。宁可多认一次会话失效（后果是多刷一次票），也不要把
	// 「需要重新登录」静默吞成一条普通失败。
	if looksLikeLoginPage(raw) {
		return true
	}
	lower := strings.ToLower(string(raw))
	for _, marker := range []string{"iap/login", "authserver", "统一身份认证"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
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

	// 复用同一份 Transport / Jar，但临时禁用跟随。
	// Transport 已套诊断，所以这里同样不需要手工埋点。
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
