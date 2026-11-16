// 诊断埋点：cpdaily 全部 HTTP 流量的统一记录点。
//
// 用法（client.go 内）：请求头设置完毕后 `rec := c.diagStart(ctx, req)`；
// 请求出错 `rec.finish("", err)`；成功 `rec.wrap(resp)`——响应体被镜像
// 进缓冲，调用方 Close 时整条请求/响应写入 diag 日志。所有脱敏在
// internal/diag 包内完成，这里只负责采集。
package cpdaily

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"qjrxy/internal/diag"
)

// diagStart 在诊断开启时为一次请求创建记录器；关闭时返回 nil（零开销）。
func (c *Client) diagStart(ctx context.Context, req *http.Request) *diagRec {
	if !diag.Enabled() || req == nil {
		return nil
	}
	r := &diagRec{
		start: time.Now(),
		entry: diag.Entry{
			TS:      time.Now().Format("2006-01-02T15:04:05.000"),
			Label:   c.label,
			TraceID: traceIDFrom(ctx),
			Method:  req.Method,
			URL:     req.URL.String(),
		},
	}
	r.entry.ReqHeaders = diag.SanitizeHeaders(req.Header)
	// body 由 NewRequestWithContext 从 bytes.Reader/Buffer 构造时自带
	// GetBody，可以无损重读；流式体拿不到就记 <stream>。
	if req.GetBody != nil {
		if bb, err := req.GetBody(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(bb, 8<<10))
			r.entry.ReqBody = diag.SanitizeBody(b, 8<<10)
		}
	} else if req.Body != nil {
		r.entry.ReqBody = "<stream>"
	}
	return r
}

// wrap 给响应体挂镜像；调用方读完 Close 时整条记录落盘。
func (r *diagRec) wrap(resp *http.Response) {
	r.entry.Status = resp.StatusCode
	capw := &cappedWriter{max: 64 << 10}
	resp.Body = &diagBody{
		Reader: io.TeeReader(resp.Body, capw),
		orig:   resp.Body,
		rec:    r,
		capw:   capw,
	}
}

// finish 直接落盘一条记录（请求阶段出错、拿不到响应时）。
func (r *diagRec) finish(respBody string, err error) {
	if r == nil {
		return
	}
	r.entry.Ms = time.Since(r.start).Milliseconds()
	r.entry.RespBody = diag.TruncateBody(respBody, 8<<10)
	if err != nil {
		r.entry.Err = err.Error()
	}
	diag.Record(r.entry)
}

// diagRec 是单次请求的进行中记录。
type diagRec struct {
	entry diag.Entry
	start time.Time
}

// diagBody 镜像响应体读取内容，Close 时写日志。
type diagBody struct {
	io.Reader
	orig io.ReadCloser
	rec  *diagRec
	capw *cappedWriter
}

func (d *diagBody) Close() error {
	d.rec.entry.Ms = time.Since(d.rec.start).Milliseconds()
	d.rec.entry.RespBody = diag.TruncateBody(d.capw.buf.String(), 8<<10)
	diag.Record(d.rec.entry)
	return d.orig.Close()
}

// cappedWriter 只保留前 max 字节，写满后继续吞掉但不报错——
// 绝不能因为诊断日志影响真实响应流。
type cappedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if w.buf.Len() < w.max {
		if room := w.max - w.buf.Len(); len(p) > room {
			w.buf.Write(p[:room])
		} else {
			w.buf.Write(p)
		}
	}
	return len(p), nil
}

// traceIDFrom 从 context 里取追踪 ID（service 层在执行任务时注入）。
func traceIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

type traceIDKey struct{}

// WithTraceID 把追踪 ID 注入 context，诊断日志用它把一次任务的
// 多个请求串起来。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// ---------- Transport 级埋点 ----------
//
// 上面那套 diagStart/wrap 要求调用方手工在 client.do 里埋点，结果是：
// 任何绕过 client.do 的请求（CAS 密码登录提交、短信验证码、账号体系补签、
// 换票、学校列表）都不会进诊断日志。2026-11-16 排查安徽师范大学登录失败时，
// 日志里只有两个 GET、没有任何 POST，根本看不出走到哪一步失败。
//
// 改成在 Transport 层统一拦截：只要请求经过这里的 http.Client 就一定会被
// 记录，以后新写的代码也不会再漏。

// flowKey 标识这条请求属于哪条业务链路，写进诊断记录的 flow 字段。
// 排查时先按 flow 分组，比在几千条混合流量里翻 URL 快得多。
type flowKey struct{}

// 业务链路名。写死成常量而不是随手传字符串，避免同一个链路在不同调用点
// 出现 "password-login" / "pwdLogin" 之类无法聚类的拼写。
const (
	FlowPasswordLogin  = "password-login"  // CAS/云平台账密登录
	FlowSMSCode        = "sms-code"        // 账号体系短信：发码
	FlowSMSLogin       = "sms-login"       // 账号体系短信：登录
	FlowSMSRefreshTGC  = "sms-tgc"         // 账号体系 sessionToken 补签 CAS 票
	FlowTicketExchange = "ticket-exchange" // CAS 票换业务会话
	FlowScanLogin      = "scan-login"      // 扫码登录取 OAuth 参数
	FlowSchoolList     = "school-list"     // 学校列表/入口解析
)

// WithFlow 给 context 标注业务链路名。同一 ctx 派生出的请求都会带上这个
// 标记；未标注时不写该字段。
func WithFlow(ctx context.Context, flow string) context.Context {
	if strings.TrimSpace(flow) == "" {
		return ctx
	}
	return context.WithValue(ctx, flowKey{}, flow)
}

// flowFrom 取上下文里的链路标记。
func flowFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(flowKey{}).(string); ok {
		return v
	}
	return ""
}

// labelKey 是上下文里的流量标识（如 "account:22"）。
//
// 账号体系短信这类链路没有 Client 承载 label（它们是包级函数），
// 只能靠 context 传递，否则诊断记录里分不清是谁的请求。
type labelKey struct{}

// WithLabel 给 context 标注流量标识。空串忽略。
func WithLabel(ctx context.Context, label string) context.Context {
	if strings.TrimSpace(label) == "" {
		return ctx
	}
	return context.WithValue(ctx, labelKey{}, label)
}

// labelFrom 取上下文里的流量标识。
func labelFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(labelKey{}).(string); ok {
		return v
	}
	return ""
}

// resolveLabel 优先用 Client 自己的 label，其次用 context 里的。
func resolveLabel(clientLabel string, ctx context.Context) string {
	if strings.TrimSpace(clientLabel) != "" {
		return clientLabel
	}
	return labelFrom(ctx)
}

// diagTransport 包在真正的 Transport 外面，为每个请求记录诊断。
// 诊断关闭时只是透传一层，开销是一次 Enabled() 读锁。
type diagTransport struct {
	base  http.RoundTripper
	label string
}

// WrapTransport 给任意 Transport 套上诊断记录。
// 传入 nil 时用 http.DefaultTransport；label 是诊断里的流量标识
// （如 "account:22"），空串表示不带。
func WrapTransport(base http.RoundTripper, label string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &diagTransport{base: base, label: label}
}

// RoundTrip 实现 http.RoundTripper。
func (t *diagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !diag.Enabled() || req == nil {
		return t.base.RoundTrip(req)
	}
	ctx := req.Context()
	rec := &diagRec{
		start: time.Now(),
		entry: diag.Entry{
			TS:      time.Now().Format("2006-01-02T15:04:05.000"),
			Label:   resolveLabel(t.label, ctx),
			TraceID: traceIDFrom(ctx),
			Flow:    flowFrom(ctx),
			Method:  req.Method,
			URL:     req.URL.String(),
		},
	}
	rec.entry.ReqHeaders = diag.SanitizeHeaders(req.Header)
	// 请求体在 RoundTrip 阶段只能靠 GetBody 无损重读；没有就记 <stream>。
	// http.NewRequestWithContext 对 bytes.Reader/Buffer 会自动设 GetBody，
	// 我们的请求体全部来自这两种，所以基本都能记到。
	if req.GetBody != nil {
		if bb, err := req.GetBody(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(bb, 8<<10))
			rec.entry.ReqBody = diag.SanitizeBody(b, 8<<10)
		}
	} else if req.Body != nil {
		rec.entry.ReqBody = "<stream>"
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		rec.finish("", err)
		return nil, err
	}
	rec.wrap(resp)
	return resp, nil
}

// HTTPClient 造一个带诊断的 http.Client，供不走 Client 封装的链路使用
// （账号体系短信、学校列表等各自需要不同的 Jar/超时/重定向策略）。
//
// timeout<=0 用 20 秒；transport 为 nil 用 http.DefaultTransport；
// checkRedirect 为 nil 时跟随默认策略。label 用于诊断分组。
//
// 注意 transport 参数在判空前先转成接口会踩 typed-nil 的坑
// （(*http.Transport)(nil) != nil），所以这里显式判指针。
func HTTPClient(label string, timeout time.Duration, transport *http.Transport, checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	var rt http.RoundTripper
	if transport != nil {
		rt = transport
	}
	return HTTPClientWithRoundTripper(label, timeout, rt, checkRedirect)
}

// HTTPClientWithRoundTripper 同 HTTPClient，但接受任意 RoundTripper。
func HTTPClientWithRoundTripper(label string, timeout time.Duration, rt http.RoundTripper, checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     WrapTransport(rt, label),
		CheckRedirect: checkRedirect,
	}
}
