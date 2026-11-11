package events

import (
	"testing"
	"time"
)

// nextEvent 在超时时间内读取一条事件。
func nextEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("等待事件超时")
		return Event{}
	}
}

// TestPublishScopedByUser 校验事件只到达对应用户。
func TestPublishScopedByUser(t *testing.T) {
	h := New()
	ch1, cancel1 := h.Subscribe(1)
	defer cancel1()
	ch2, cancel2 := h.Subscribe(2)
	defer cancel2()

	h.Publish(2, TopicLogs)

	select {
	case ev := <-ch1:
		t.Fatalf("用户 1 不应收到用户 2 的事件：%+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
	if ev := nextEvent(t, ch2); len(ev.Topics) != 1 || ev.Topics[0] != TopicLogs {
		t.Fatalf("用户 2 收到错误主题：%+v", ev)
	}
}

// TestPublishDedupesTopics 校验同一事件里的重复主题会被合并。
func TestPublishDedupesTopics(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe(1)
	defer cancel()

	h.Publish(1, TopicTasks, TopicTasks, "", TopicLogs)
	ev := nextEvent(t, ch)
	if len(ev.Topics) != 2 || ev.Topics[0] != TopicTasks || ev.Topics[1] != TopicLogs {
		t.Fatalf("主题去重结果异常：%+v", ev.Topics)
	}
}

// TestPublishBroadcast 校验 userID=0 的广播会到达所有订阅者。
func TestPublishBroadcast(t *testing.T) {
	h := New()
	ch1, cancel1 := h.Subscribe(1)
	defer cancel1()
	ch2, cancel2 := h.Subscribe(2)
	defer cancel2()

	h.Publish(0, TopicPlans)
	for _, ch := range []<-chan Event{ch1, ch2} {
		if ev := nextEvent(t, ch); len(ev.Topics) != 1 || ev.Topics[0] != TopicPlans {
			t.Fatalf("广播事件异常：%+v", ev)
		}
	}
}

// TestOverflowFallsBackToAll 校验消费过慢时用 all 主题兜底，而不是阻塞发布方。
func TestOverflowFallsBackToAll(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe(1)
	defer cancel()

	// 填满缓冲后再发一条：最旧的一条被丢弃，队尾换成 all。
	for i := 0; i < subscriptionBuffer+1; i++ {
		h.Publish(1, TopicLogs)
	}
	// 缓冲区始终是 subscriptionBuffer 条：前 subscriptionBuffer-1 条是原事件，
	// 最后一条是 all（第 17 次发布丢掉了最旧的一条）。
	for i := 0; i < subscriptionBuffer-1; i++ {
		nextEvent(t, ch)
	}
	ev := nextEvent(t, ch)
	if len(ev.Topics) != 1 || ev.Topics[0] != TopicAll {
		t.Fatalf("溢出兜底主题异常：%+v", ev.Topics)
	}
}

// TestUnsubscribe 校验取消订阅后连接被移除且不再收到事件。
func TestUnsubscribe(t *testing.T) {
	h := New()
	_, cancel := h.Subscribe(1)
	if got := h.Subscribers(); got != 1 {
		t.Fatalf("订阅数应为 1，得到 %d", got)
	}
	cancel()
	if got := h.Subscribers(); got != 0 {
		t.Fatalf("取消后订阅数应为 0，得到 %d", got)
	}
}
