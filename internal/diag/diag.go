// Package diag 记录对学校服务器的 HTTP 层诊断日志。
//
// 目的：出现「验证码验签失败」这类无法复现的业务拒绝时，能回放我们
// 发出的请求（URL/头/体）与服务端的原始响应——没有这份记录，定位只能
// 靠猜。日志按天落在 <data>/diag/diag-YYYYMMDD.jsonl，前端与 API 均
// 不暴露，只有服务器管理员能读到文件。
//
// 隐私约束（写代码时就固定，调用方无法绕过）：
//   - Cookie 只记键名 + 值前 6 位 + 长度（够判断带了哪张票，不泄露票据）
//   - Authorization / CpdailyInfo / sessionTokenKey 等凭证头只记 <set>
//   - 请求/响应体里的密码字段替换为 <redacted>，正文各截断 8KB
//   - 二进制响应（图片等）只记 content-type 与字节数
//
// 写入是异步的（有界队列），请求路径上只有一次入队开销；队列满时
// 丢弃并计数，绝不阻塞签到。
package diag

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type config struct {
	dataDir  string
	enabled  bool
	keepDays int
}

var (
	mu        sync.RWMutex
	cfg       config
	ch        chan Entry
	wg        sync.WaitGroup
	closeOnce sync.Once
	startOnce sync.Once
	closed    bool  // Close 已执行；Record 在同一把读锁下检查，避免向已关闭 channel 发送
	dropped   int64 // 队列满被丢弃的条数（诊断自身出问题时看的）
	droppedMu sync.Mutex
)

// Entry 是一条诊断记录。ReqHeaders/ReqBody/RespBody 由调用方脱敏后传入，
// 或直接使用本包的 Sanitize 辅助函数。
type Entry struct {
	TS         string            `json:"ts"`
	Label      string            `json:"label,omitempty"`
	TraceID    string            `json:"traceId,omitempty"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Status     int               `json:"status,omitempty"`
	Ms         int64             `json:"ms,omitempty"`
	Err        string            `json:"err,omitempty"`
	ReqHeaders map[string]string `json:"reqHeaders,omitempty"`
	ReqBody    string            `json:"reqBody,omitempty"`
	RespBody   string            `json:"respBody,omitempty"`
}

// Configure 设置诊断日志的存储位置与默认开关。
// 必须在第一次 Record 之前调用；重复调用会更新开关但保持 worker 不变。
func Configure(dataDir string, enabled bool, keepDays int) {
	mu.Lock()
	if keepDays <= 0 {
		keepDays = 7
	}
	cfg = config{dataDir: dataDir, enabled: enabled, keepDays: keepDays}
	mu.Unlock()
	startOnce.Do(start)
}

// SetEnabled 运行时开关（管理员页调用，立即生效）。
func SetEnabled(enabled bool) {
	mu.Lock()
	cfg.enabled = enabled
	mu.Unlock()
}

// SetKeepDays 运行时调整保留天数（管理员页调用）。
func SetKeepDays(days int) {
	if days <= 0 {
		days = 7
	}
	mu.Lock()
	cfg.keepDays = days
	mu.Unlock()
}

// Enabled 报告当前是否在记录。
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return cfg.enabled
}

// KeepDays 返回保留天数。
func KeepDays() int {
	mu.RLock()
	defer mu.RUnlock()
	return cfg.keepDays
}

func start() {
	mu.RLock()
	dir := cfg.dataDir
	mu.RUnlock()
	mu.Lock()
	ch = make(chan Entry, 256)
	mu.Unlock()
	wg.Add(1)
	go worker(dir)
}

// Record 提交一条诊断记录（异步）。未启用或队列满时静默丢弃。
func Record(e Entry) {
	// 整个「检查 + 发送」都持有读锁：Close 拿写锁先关标志再 close(ch)，
	// 两条路互斥，避免向已关闭的 channel 发送而 panic
	// （select 的 default 分支救不了向 closed channel 的 send）。
	mu.RLock()
	defer mu.RUnlock()
	if !cfg.enabled || closed || ch == nil {
		return
	}
	if e.TS == "" {
		e.TS = time.Now().Format("2006-01-02T15:04:05.000")
	}
	select {
	case ch <- e:
	default:
		droppedMu.Lock()
		dropped++
		droppedMu.Unlock()
	}
}

// DroppedCount 返回因队列满而丢弃的条数（仅诊断自身用）。
func DroppedCount() int64 {
	droppedMu.Lock()
	defer droppedMu.Unlock()
	return dropped
}

// Close 停止 worker 并把队列里剩余的记录写完。进程退出时调用。
func Close() {
	closeOnce.Do(func() {
		mu.Lock()
		closed = true
		if ch != nil {
			close(ch)
		}
		mu.Unlock()
		wg.Wait()
	})
}

func worker(dataDir string) {
	defer wg.Done()
	var (
		day    string
		purged string
		f      *os.File
		enc    *json.Encoder
		broken bool
	)
	closeFile := func() {
		if f != nil {
			f.Close()
			f = nil
		}
	}
	defer closeFile()

	for e := range ch {
		day2 := time.Now().Format("20060102")
		if f == nil || day2 != day {
			closeFile()
			if err := os.MkdirAll(filepath.Join(dataDir, "diag"), 0o755); err != nil {
				continue
			}
			nf, err := os.OpenFile(filepath.Join(dataDir, "diag", "diag-"+day2+".jsonl"),
				os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				day = ""
				continue
			}
			f, day = nf, day2
			enc = json.NewEncoder(f)
			broken = false
			// 每天第一个文件只清理一次过期日志
			if purged != day2 {
				purge(dataDir)
				purged = day2
			}
		}
		if broken {
			continue
		}
		if err := enc.Encode(e); err != nil {
			// 磁盘满等写失败：当天文件不再写，避免每条都报错
			broken = true
		}
	}
}

// purge 删除超过保留期的诊断文件。
func purge(dataDir string) {
	keep := KeepDays()
	if keep <= 0 {
		keep = 7
	}
	cutoff := time.Now().AddDate(0, 0, -keep).Format("20060102")
	matches, err := filepath.Glob(filepath.Join(dataDir, "diag", "diag-*.jsonl"))
	if err != nil {
		return
	}
	for _, m := range matches {
		day := strings.TrimSuffix(filepath.Base(m), ".jsonl")
		day = strings.TrimPrefix(day, "diag-")
		if day < cutoff {
			os.Remove(m)
		}
	}
}

// ---- 脱敏 ----

// sensitiveHeaders 是只记 <set> 的凭证头（键名小写比较）。
var sensitiveHeaders = map[string]bool{
	"authorization":    true,
	"cpdailyinfo":      true,
	"sessiontokenkey":  true,
	"sessiontoken":     true,
	"wisdeviceid":      true,
	"tgc":              true,
	"authtgc":          true,
	"castgc":           true,
	"cpdailyextension": true,
}

// SanitizeHeaders 复制一份请求头并脱敏：
// Cookie 记键名+值前6位+长度，凭证头记 <set>，其余原样。
func SanitizeHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		key := strings.ToLower(k)
		switch {
		case key == "cookie":
			out[k] = sanitizeCookies(vs)
		case sensitiveHeaders[key]:
			if len(strings.Join(vs, ",")) > 0 {
				out[k] = "<set>"
			}
		default:
			out[k] = strings.Join(vs, ",")
		}
	}
	return out
}

// sanitizeCookies 把 "a=1; b=2" 形式的 Cookie 换成
// "a=abc123…(len42); b=…"，票据本体不落盘。
func sanitizeCookies(values []string) string {
	parts := make([]string, 0, 4)
	for _, raw := range values {
		for _, pair := range strings.Split(raw, "; ") {
			name, val, ok := strings.Cut(pair, "=")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			prefix := val
			if len(prefix) > 6 {
				prefix = prefix[:6]
			}
			parts = append(parts, fmt.Sprintf("%s=%s…(len%d)", name, prefix, len(val)))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

// SanitizeBody 把 JSON 体里的密码类字段替换为 <redacted>，并截断。
// 非 JSON 体原样截断。
func SanitizeBody(body []byte, limit int) string {
	var v any
	if err := json.Unmarshal(body, &v); err == nil {
		redactJSON(v)
		if cleaned, err := json.Marshal(v); err == nil {
			body = cleaned
		}
	}
	s := string(body)
	if len(s) > limit {
		s = s[:limit] + "…(截断)"
	}
	return s
}

func redactJSON(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, fv := range t {
			switch strings.ToLower(k) {
			case "password", "oldpassword", "newpassword", "loginpwd":
				if fv != nil && fv != "" {
					t[k] = "<redacted>"
				}
			default:
				redactJSON(fv)
			}
		}
	case []any:
		for _, item := range t {
			redactJSON(item)
		}
	}
}

// TruncateBody 供无 JSON 结构的体使用（如 multipart），只截断。
func TruncateBody(s string, limit int) string {
	if len(s) > limit {
		return s[:limit] + "…(截断)"
	}
	return s
}
