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
