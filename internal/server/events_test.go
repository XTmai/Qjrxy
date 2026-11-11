package server_test

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"qjrxy/internal/events"
)

// readSSELine 在超时时间内读到包含 want 的行。
func readSSELine(t *testing.T, lines <-chan string, want string) string {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("连接提前结束，未找到 %q", want)
			}
			if strings.Contains(line, want) {
				return line
			}
		case <-deadline:
			t.Fatalf("等待 SSE 行 %q 超时", want)
		}
	}
}

// TestEventsStream 校验 SSE 鉴权、就绪事件与变更推送。
func TestEventsStream(t *testing.T) {
	env := newEnv(t)
	user, err := env.svc.Authenticate(context.Background(), env.token)
	if err != nil {
		t.Fatalf("取当前用户失败：%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+env.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("连接事件流失败：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("事件流应返回 200，得到 %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type 应为 text/event-stream，得到 %q", ct)
	}
	// 反向代理必须看到这个头才会关闭缓冲。
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering 应为 no，得到 %q", got)
	}

	lines := make(chan string, 32)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	readSSELine(t, lines, "event: ready")

	env.svc.Events().Publish(user.ID, events.TopicLogs, events.TopicTasks)
	readSSELine(t, lines, "event: invalidate")
	data := readSSELine(t, lines, "data: ")
	if !strings.Contains(data, `"logs"`) || !strings.Contains(data, `"tasks"`) {
		t.Fatalf("事件主题异常：%s", data)
	}

	// 断线后订阅应立即释放，避免长期占用连接表。
	cancel()
	_ = resp.Body.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if env.svc.Events().Subscribers() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("断线后订阅未释放，仍有 %d 个连接", env.svc.Events().Subscribers())
}

// TestEventsRequireAuth 校验未登录不能订阅事件流。
func TestEventsRequireAuth(t *testing.T) {
	env := newEnv(t)
	status, body := env.do(t, http.MethodGet, "/api/events", nil, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，得到 %d %v", status, body)
	}
}
