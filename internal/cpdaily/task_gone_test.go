// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNeedsCaptchaIgnoresReusedCode 校验 2210060000 被学校复用时不再误判。
//
// 回归（2026-11-17 生产）：cmoc、cumt 两个账号的 submitSign 都返回
// 2210060000，但 message 分别是「当前查寝任务不支持手机签到，请前往nfc
// 机器位置签到！」和「签到身份不一致，请重新登录保持签到身份一致！」，
// checkValidation 均为 false。旧实现只看 code 就判定需要验证码，于是
// 白跑 4 轮九宫格（每轮下载 9 张图 + 换票），最后界面还显示一句
// 「请先在管理员设置里配置识别服务」——与真实原因完全无关。
func TestNeedsCaptchaIgnoresReusedCode(t *testing.T) {
	cases := []struct {
		name string
		res  *SignResult
		want bool
	}{
		{
			name: "NFC 任务不支持手机签到",
			res:  &SignResult{Code: captchaRequiredCode, Message: "当前查寝任务不支持手机签到，请前往nfc机器位置签到！"},
			want: false,
		},
		{
			name: "签到身份不一致",
			res:  &SignResult{Code: captchaRequiredCode, Message: "签到身份不一致，请重新登录保持签到身份一致！"},
			want: false,
		},
		{
			name: "真正的验证码验签失败",
			res:  &SignResult{Code: captchaRequiredCode, Message: "验证码验签失败"},
			want: true,
		},
		{
			name: "code 不稳定但 message 提到验证码",
			res:  &SignResult{Code: "0", Message: "请先通过验证码"},
			want: true,
		},
		{
			name: "普通成功结果",
			res:  &SignResult{Success: true, Code: "0", Message: "SUCCESS"},
			want: false,
		},
		{
			name: "nil 安全",
			res:  nil,
			want: false,
		},
	}
	for _, c := range cases {
		if got := needsCaptcha(c.res); got != c.want {
			t.Errorf("%s：needsCaptcha = %v，期望 %v", c.name, got, c.want)
		}
	}
}

// TestExplainFailureForReusedCode 校验复用码的提示指向真实原因。
func TestExplainFailureForReusedCode(t *testing.T) {
	cases := []struct {
		code, msg, wantContains string
		notContains             string
	}{
		{
			code: captchaRequiredCode,
			msg:  "当前查寝任务不支持手机签到，请前往nfc机器位置签到！",
			// 必须说清「只能去现场」，并且不能再提识别服务。
			wantContains: "NFC",
			notContains:  "识别服务",
		},
		{
			code:         captchaRequiredCode,
			msg:          "签到身份不一致，请重新登录保持签到身份一致！",
			wantContains: "重新登录",
			notContains:  "识别服务",
		},
		{
			code:         captchaRequiredCode,
			msg:          "验证码验签失败",
			wantContains: "验证码",
		},
	}
	for _, c := range cases {
		got := ExplainFailure(c.code, c.msg)
		if !strings.Contains(got, c.wantContains) {
			t.Errorf("ExplainFailure(%q,%q) = %q，应包含 %q", c.code, c.msg, got, c.wantContains)
		}
		if c.notContains != "" && strings.Contains(got, c.notContains) {
			t.Errorf("ExplainFailure(%q,%q) = %q，不应包含 %q", c.code, c.msg, got, c.notContains)
		}
	}
}

// TestFetchDetailMarksDeletedTask 校验任务被学校删除时返回 ErrTaskGone。
//
// 回归（2026-11-17 三峡大学科技学院）：学校在 16:00 后把旧任务删掉重发，
// signInstanceWid 从 41461 变成 41488。缓存里仍是旧 wid 的 4 个账号在
// 21:00 全部拿到 code=5「该查寝已不存在，可能已被删除！」，当天直接失败；
// 手动执行之所以成功，只是因为手动路径重拉了任务列表。
func TestFetchDetailMarksDeletedTask(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "5",
			"message": "该查寝已不存在，可能已被删除！",
			"datas":   nil,
		})
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	_, err := c.FetchDetail(context.Background(), strings.TrimPrefix(srv.URL, "https://"),
		KindAttendance, Task{Kind: KindAttendance, SignInstanceWID: "41461", SignWID: "200528216"})
	if err == nil {
		t.Fatal("任务已被删除时应返回错误")
	}
	if !errors.Is(err, ErrTaskGone) {
		t.Fatalf("应返回 ErrTaskGone，实际 %v", err)
	}
	// 错误信息里要带旧 wid，便于从日志直接对上缓存里那条任务。
	if !strings.Contains(err.Error(), "41461") {
		t.Errorf("错误信息应包含旧 wid，实际 %q", err.Error())
	}
}

// TestFetchDetailKeepsOtherErrors 校验其它业务错误不被误判成任务删除。
func TestFetchDetailKeepsOtherErrors(t *testing.T) {
	cases := []struct{ code, msg string }{
		{"0", "SUCCESS"}, // 占位，下面只测错误码
		{"22100100000", "查寝任务wid不能为空"},
		{"500", "服务器内部错误"},
	}
	for _, c := range cases[1:] {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": c.code, "message": c.msg})
		}))
		hc := srv.Client()
		hc.Timeout = 5 * time.Second
		client := NewClient(ClientOptions{HTTPClient: hc}, nil)
		_, err := client.FetchDetail(context.Background(), strings.TrimPrefix(srv.URL, "https://"),
			KindAttendance, Task{SignInstanceWID: "1", SignWID: "2"})
		srv.Close()
		if err == nil {
			t.Fatalf("code=%s 应返回错误", c.code)
		}
		if errors.Is(err, ErrTaskGone) {
			t.Errorf("code=%s 不应被判定为任务删除，实际 %v", c.code, err)
		}
	}
}
