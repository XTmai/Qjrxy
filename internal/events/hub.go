// Package events 提供服务端到浏览器的轻量变更广播（SSE）。
//
// 只广播「哪类数据变了」，不携带业务数据：前端收到后让对应查询失效并
// 重新拉取，权限过滤与字段裁剪仍然只走各自的 HTTP 接口。
// 这样事件体极小，也不会因为推送绕过鉴权。
package events

import (
	"sync"
	"time"
)

// 主题常量：与前端 queryKeys 的根节点一一对应。
const (
	TopicAccounts = "accounts"
	TopicTasks    = "tasks"
	TopicPlans    = "plans"
	TopicLogs     = "logs"
	// TopicCaptcha 表示「有验证码在等用户点」。前端收到后拉一次待处理列表，
	// 不把题目数据塞进事件体——它可能是一百多 KB 的图片。
	TopicCaptcha = "captcha"

	// TopicAll 是订阅端缓冲溢出时的兜底主题：前端收到后全量失效。
	TopicAll = "all"
)

// Event 是一条变更通知。
type Event struct {
	Topics []string  `json:"topics"`
	At     time.Time `json:"at"`
}

// Hub 维护所有在线连接，并把变更按用户分发。
//
// 生产环境通常只有一两个连接（单用户本地部署），但多标签页与
// 多用户部署下同一进程内的连接数会更多，因此分发必须是并发安全的。
type Hub struct {
	mu   sync.Mutex
	next int
	subs map[int]*subscriber
}

type subscriber struct {
	userID int64
	ch     chan Event
}

// subscriptionBuffer 是单连接的待发事件上限。
//
// 正常一轮自动查寝只会产生个位数事件，16 足够；溢出说明页面长时间
// 没有消费（后台标签页被冻结等），此时丢掉旧事件并改发 TopicAll，
// 让前端一次性对齐，而不是积压几百条细粒度通知。
const subscriptionBuffer = 16

// New 创建事件中心。
func New() *Hub {
	return &Hub{subs: map[int]*subscriber{}}
}

// Subscribe 订阅指定用户的变更，返回事件通道与取消函数。
//
// userID 为 0 的订阅者接收全部事件（主要用于测试与诊断）。
func (h *Hub) Subscribe(userID int64) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[int]*subscriber{}
	}
	id := h.next
	h.next++
	sub := &subscriber{userID: userID, ch: make(chan Event, subscriptionBuffer)}
	h.subs[id] = sub

	cancel := func() {
		h.mu.Lock()
		delete(h.subs, id)
		h.mu.Unlock()
	}
	return sub.ch, cancel
}

// Publish 向指定用户广播变更；userID 为 0 表示全体。
//
// 不阻塞调用方：某个连接消费不动时只影响它自己，不能拖住正在写库的
// 业务协程（调度器与 HTTP 请求都会调用这里）。
func (h *Hub) Publish(userID int64, topics ...string) {
	topics = dedupe(topics)
	if len(topics) == 0 {
		return
	}
	ev := Event{Topics: topics, At: time.Now()}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		if userID != 0 && sub.userID != 0 && sub.userID != userID {
			continue
		}
		if deliver(sub.ch, ev) {
			continue
		}
		// 缓冲满：丢掉一条最旧的事件，用 TopicAll 占位，前端据此全量刷新。
		select {
		case <-sub.ch:
		default:
		}
		deliver(sub.ch, Event{Topics: []string{TopicAll}, At: ev.At})
	}
}

// Subscribers 返回当前连接数（诊断与测试用）。
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func deliver(ch chan Event, ev Event) bool {
	select {
	case ch <- ev:
		return true
	default:
		return false
	}
}

func dedupe(topics []string) []string {
	seen := make(map[string]struct{}, len(topics))
	out := make([]string, 0, len(topics))
	for _, t := range topics {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
