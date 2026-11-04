package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter 是按 key 计数的固定窗口限流器，用于登录/注册防爆破。
//
// 只依赖进程内状态：单实例部署足够。多副本部署时每个副本各自计数，
// 实际放行量会按副本数放大——这是有意的取舍，宁可放宽也不误伤正常用户。
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*rateEntry
}

type rateEntry struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, entries: make(map[string]*rateEntry)}
}

// allow 记录一次尝试；超限时返回需要等待的时长。
func (l *rateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if e, ok := l.entries[key]; ok && now.Before(e.reset) {
		if e.count >= l.limit {
			return false, e.reset.Sub(now)
		}
		e.count++
		return true, 0
	}
	l.entries[key] = &rateEntry{count: 1, reset: now.Add(l.window)}
	l.pruneLocked(now)
	return true, 0
}

// reset 清空某个 key 的计数（登录成功后调用，避免正常用户被自己之前的手误锁住）。
func (l *rateLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// pruneLocked 在条目变多时清理过期项，避免 map 无界增长。
func (l *rateLimiter) pruneLocked(now time.Time) {
	if len(l.entries) < 2048 {
		return
	}
	for k, e := range l.entries {
		if now.After(e.reset) {
			delete(l.entries, k)
		}
	}
}

// authLimitKey 生成限流键：客户端 IP + 账号名。
//
// 只取 RemoteAddr，不读 X-Forwarded-For：该头可被客户端伪造，
// 用它做限流键等于给攻击者留一个随意重置计数的开关。
// 部署在反向代理后面时，所有请求的 IP 会退化为代理地址，
// 此时限流退化为"按账号名"生效，仍然能挡住针对单账号的撞库。
func authLimitKey(r *http.Request, username string) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return host + "|" + strings.ToLower(strings.TrimSpace(username))
}

// tooMany 构造 429 响应错误。
func tooMany(wait time.Duration) error {
	secs := int(wait.Seconds())
	if secs < 1 {
		secs = 1
	}
	return &APIError{
		Status:  http.StatusTooManyRequests,
		Code:    "too_many_requests",
		Message: fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", secs),
	}
}
