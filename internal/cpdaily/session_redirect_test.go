// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPostJSON405SessionRedirect 校验「302 → 登录页 → 405」被识别成会话失效。
//
// 回归（2026-11-17 安徽师范 26111702060）：业务会话 16:02 失效后，每小时
// 的拉取都是这条链——POST 业务接口 302 → GET /iap/login 302 → GET 业务路径
// 405（Apache Tomcat 错误页）。405 的 body 一个会话失效关键词都没有，
// IsAuthExpired 返回 false，于是：
//   - SyncTasks 的自动恢复分支（auth.go）不进入；
//   - 用户只在界面上看到一条普通失败，「拉不到任务」却没有任何指引。
//
// 修复后 405 会被改写成 401 语义，IsAuthExpired 能识别并触发续期/重登。
func TestPostJSON405SessionRedirect(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/portal/login", func(w http.ResponseWriter, r *http.Request) {
		// 业务路径：未认证时 302 到 IAP 登录页
		http.Redirect(w, r, "/iap/login?service=%2Fbiz", http.StatusFound)
	})
	mux.HandleFunc("/iap/login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/biz", http.StatusFound)
	})
	mux.HandleFunc("/biz", func(w http.ResponseWriter, r *http.Request) {
		// 重定向把 POST 改成了 GET，落到只接受 POST 的业务路径上
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Apache Tomcat/8.0.50 - Error report</title></head><body>HTTP Status 405</body></html>`))
	})

	srv := httptest.NewTLSServer(&mux)
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	host := stripScheme(srv.URL)

	_, err := c.postJSON(context.Background(), "https://"+host+"/portal/login", nil, nil)
	if err == nil {
		t.Fatal("这条链最终是 405，应返回错误")
	}
	if !IsAuthExpired(err) {
		t.Fatalf("405 来自登录页重定向，应判定为会话失效；实际错误：%v", err)
	}
}

// TestPostJSON405RealMethodError 校验真正的 405 不被误判成会话失效。
//
// 业务接口对合法会话也会因为方法不对返回 405。这种情况下不该触发重登，
// 否则会拿用户密码去撞登录接口。
func TestPostJSON405RealMethodError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("method not allowed"))
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	_, err := c.postJSON(context.Background(), srv.URL+"/wec-counselor-attendance-apps/x", nil, nil)
	if err == nil {
		t.Fatal("405 应返回错误")
	}
	if IsAuthExpired(err) {
		t.Fatalf("普通 405 不应判定为会话失效：%v", err)
	}
}

// TestPostJSONLoginPageStillExpired 校验 200 返回登录页仍按会话失效处理
// （这是原有行为，改动 405 判定后必须保持）。
func TestPostJSONLoginPageStillExpired(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><form id="casLoginForm">统一身份认证</form></body></html>`))
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	_, err := c.postJSON(context.Background(), srv.URL+"/wec-counselor-attendance-apps/x", nil, nil)
	if err == nil {
		t.Fatal("返回登录页 HTML 时应报错")
	}
	if !IsAuthExpired(err) {
		t.Fatalf("登录页应判定为会话失效：%v", err)
	}
}

// TestFetchTaskListsSingleRequest 校验列表接口只发一次请求。
//
// 历史上这里连着发两次、只取第二次结果，注释称「第一次用于获取 cookie」，
// 但会话票据本来就由 client 显式携带。diag 统计：全天 16113 对这类重复
// 请求响应逐字节相同，仅有的差异对里还有一次是第一跳成功、第二跳返回
// 登录页——重复请求除了翻倍流量，还会把一个成功的同步变成失败。
func TestFetchTaskListsSingleRequest(t *testing.T) {
	var calls int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "0",
			"message": "SUCCESS",
			"datas": map[string]any{
				"dayInMonth":    "2026-11-17",
				"unSignedTasks": []any{},
				"signedTasks":   []any{},
				"leaveTasks":    []any{},
			},
		})
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	if _, err := c.fetchTaskLists(context.Background(), stripScheme(srv.URL), KindAttendance); err != nil {
		t.Fatalf("拉取列表不应失败：%v", err)
	}
	if calls != 1 {
		t.Errorf("列表接口应只请求 1 次，实际 %d 次", calls)
	}
}

func stripScheme(u string) string {
	for _, p := range []string{"https://", "http://"} {
		if len(u) > len(p) && u[:len(p)] == p {
			return u[len(p):]
		}
	}
	return u
}
