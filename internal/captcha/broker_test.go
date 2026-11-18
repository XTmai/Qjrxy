// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package captcha

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func openTest(b *Broker, userID int64) *Pending {
	return b.Open(userID, 7, Request{Name: "自行车", Grid: "data:image/jpeg;base64,AAA", Cells: 9})
}

// 用户回传后 Wait 立刻返回，且格子按升序去重。
func TestBrokerSubmitWakesWait(t *testing.T) {
	b := NewBroker(time.Minute)
	p := openTest(b, 1)

	var (
		got     []int
		waitErr error
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, waitErr = b.Wait(context.Background(), p)
	}()

	time.Sleep(10 * time.Millisecond)
	if err := b.Submit(1, p.Token, []int{5, 2, 5}); err != nil {
		t.Fatalf("提交失败：%v", err)
	}
	wg.Wait()

	if waitErr != nil {
		t.Fatalf("等待应正常结束：%v", waitErr)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 5 {
		t.Fatalf("应去重且升序得到 [2 5]，实际 %v", got)
	}
}

// 越界与空选择的处理：越界静默丢弃，全丢光要报错。
func TestBrokerSubmitRejectsBadCells(t *testing.T) {
	b := NewBroker(time.Minute)
	p := openTest(b, 1)

	if err := b.Submit(1, p.Token, []int{0, 10, -1}); !errors.Is(err, ErrNoSelection) {
		t.Fatalf("全部越界应报「请至少选择一张」，实际 %v", err)
	}
	// 报错不应消耗会话，用户可以重新点
	if err := b.Submit(1, p.Token, []int{3}); err != nil {
		t.Fatalf("重新提交应成功：%v", err)
	}
}

// 换一张与取消各自返回对应信号。
func TestBrokerSkipAndCancel(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Broker, int64, string) error
		want error
	}{
		{"skip", (*Broker).Skip, ErrSkip},
		{"cancel", (*Broker).Cancel, ErrCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBroker(time.Minute)
			p := openTest(b, 1)
			errc := make(chan error, 1)
			go func() {
				_, err := b.Wait(context.Background(), p)
				errc <- err
			}()
			time.Sleep(10 * time.Millisecond)
			if err := tc.call(b, 1, p.Token); err != nil {
				t.Fatalf("调用失败：%v", err)
			}
			select {
			case err := <-errc:
				if !errors.Is(err, tc.want) {
					t.Fatalf("应返回 %v，实际 %v", tc.want, err)
				}
			case <-time.After(time.Second):
				t.Fatal("等待未在 1s 内结束")
			}
		})
	}
}

// 超时后返回 ErrExpired，而不是一直挂着。
func TestBrokerWaitTimeout(t *testing.T) {
	b := NewBroker(40 * time.Millisecond)
	p := openTest(b, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := b.Wait(ctx, p); !errors.Is(err, ErrExpired) {
		t.Fatalf("超时应返回 ErrExpired，实际 %v", err)
	}
	// 超时后会话应被清理
	if err := b.Submit(1, p.Token, []int{1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("过期会话应不可提交，实际 %v", err)
	}
}

// 列表按用户隔离，且过滤过期项。
func TestBrokerListScopedAndFresh(t *testing.T) {
	b := NewBroker(time.Minute)
	a := openTest(b, 1)
	openTest(b, 2)

	got := b.List(1)
	if len(got) != 1 || got[0].Token != a.Token {
		t.Fatalf("用户 1 应只看到自己的会话，实际 %d 条", len(got))
	}
	if l := b.List(3); len(l) != 0 {
		t.Fatalf("无关用户不应看到会话，实际 %d 条", len(l))
	}

	// 把时钟推到过期之后
	b.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if l := b.List(1); len(l) != 0 {
		t.Fatalf("过期会话不应出现在列表里，实际 %d 条", len(l))
	}
}

// 别人的 token 不能提交，且错误与「不存在」一致，避免被拿来探测。
func TestBrokerSubmitOtherUserToken(t *testing.T) {
	b := NewBroker(time.Minute)
	p := openTest(b, 1)
	if err := b.Submit(2, p.Token, []int{1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("跨用户提交应报不存在，实际 %v", err)
	}
}

// 重复提交：第一次生效，第二次因为会话已被 Wait 收走而报不存在。
func TestBrokerSubmitTwice(t *testing.T) {
	b := NewBroker(time.Minute)
	p := openTest(b, 1)
	if err := b.Submit(1, p.Token, []int{1}); err != nil {
		t.Fatalf("首次提交失败：%v", err)
	}
	if _, err := b.Wait(context.Background(), p); err != nil {
		t.Fatalf("等待应拿到结果：%v", err)
	}
	if err := b.Submit(1, p.Token, []int{2}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("二次提交应报不存在，实际 %v", err)
	}
}

// 并发提交只应有一个生效，不能重复唤醒导致 panic 或丢结果。
func TestBrokerConcurrentSubmit(t *testing.T) {
	b := NewBroker(time.Minute)
	p := openTest(b, 1)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = b.Submit(1, p.Token, []int{n + 1})
		}(i)
	}
	wg.Wait()
	if _, err := b.Wait(context.Background(), p); err != nil {
		t.Fatalf("应能取到一次结果：%v", err)
	}
}
