// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

// 2026-11-18 生产日志暴露的三个缺陷的回归测试。
// 所有样本都取自当天真实响应，键名与取值保持原样。

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

// TestUnsupportedSignModeBlocksBeforeCaptcha 校验 NFC / 二维码任务
// 在拉详情后立刻被拦下，不会去跑验证码。
//
// 回归（2026-11-18 安徽城市管理职业学院 25231011）：任务名为
// 「每日NFC签到」、signMode=5，列表里 taskType 为 null 看不出类型。
// 旧实现在详情之后直接进提交流程：先白跑一整轮九宫格（下载 9 张图、
// 调模型识别、换 ticket），提交后才被 2210060000「请前往nfc机器位置
// 签到」拒绝。7 分钟内重复 23 轮，其中 8 次撞上验证码频控。
func TestUnsupportedSignModeBlocksBeforeCaptcha(t *testing.T) {
	var captchaCalls, submitCalls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "detailSignInstance"):
			// 真实样本：signMode=5，任务名「每日NFC签到」
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","datas":{` +
				`"signInstanceWid":"46993","signMode":5,"signRate":"1",` +
				`"taskName":"每日NFC签到","signStatus":"0"}}`))
		case strings.Contains(r.URL.Path, "captcha"):
			captchaCalls++
			_, _ = w.Write([]byte(`{"code":200,"result":{"code":"x","imageInfos":[]}}`))
		case strings.Contains(r.URL.Path, "submitSign"):
			submitCalls++
			_, _ = w.Write([]byte(`{"code":"2210060000","message":"当前查寝任务不支持手机签到，请前往nfc机器位置签到！"}`))
		default:
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","datas":{}}`))
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	res, _, err := c.Sign(context.Background(), stripScheme(srv.URL), "t1",
		DeviceProfile{DeviceID: "dev-1", UserID: "u1", Lon: 118.7, Lat: 32.0},
		SignOptions{Kind: KindAttendance, Task: Task{SignInstanceWID: "46993", SignWID: "200585087"}})
	if err != nil {
		t.Fatalf("不应返回底层错误：%v", err)
	}
	if res.Success {
		t.Fatal("NFC 任务不应被当成成功")
	}
	if res.Code != "sign_mode_nfc" {
		t.Errorf("应返回 sign_mode_nfc，实际 %q", res.Code)
	}
	if !strings.Contains(res.Hint, "NFC") {
		t.Errorf("提示应说明是 NFC 任务，实际 %q", res.Hint)
	}
	if captchaCalls != 0 {
		t.Errorf("NFC 任务不应触发验证码流程，实际调用了 %d 次", captchaCalls)
	}
	if submitCalls != 0 {
		t.Errorf("NFC 任务不应提交，实际提交 %d 次", submitCalls)
	}
}

// TestQrCodeSignModeBlocked 校验二维码任务同样被拦下。
func TestQrCodeSignModeBlocked(t *testing.T) {
	if r := unsupportedSignModeResult(signModeQRCode); r == nil {
		t.Fatal("signMode=3（二维码）应返回确定性结果")
	} else if r.Code != "sign_mode_qrcode" {
		t.Errorf("code 应为 sign_mode_qrcode，实际 %q", r.Code)
	}
}

// TestUnknownSignModeProceeds 校验未知 signMode 不被误判。
//
// 宁可多发一次请求，也不要因为出现了没见过的 signMode 就把正常任务
// 判成"不支持"——那会让本来能签的账号当天失败。
func TestUnknownSignModeProceeds(t *testing.T) {
	for _, mode := range []int{1, 2, 4, 9} {
		if r := unsupportedSignModeResult(mode); r != nil {
			t.Errorf("signMode=%d 是未知值，不应被判定为不支持（实际返回 %q）", mode, r.Code)
		}
	}
	// 0 是普通定位签到，必须放行。
	if r := unsupportedSignModeResult(signModeLocation); r != nil {
		t.Error("signMode=0 应放行")
	}
}

// TestCaptchaThrottleCodeDetected 校验 20000015 被识别为频控而非识别失败。
//
// 回归（2026-11-18）：当天 24 轮验证码里有 8 轮返回
// {"code":20000015,"message":"验证过于频繁，请稍后再试!"}，
// 旧实现把它当普通失败，5 秒后就重试，限流窗口不断重置。
func TestCaptchaThrottleCodeDetected(t *testing.T) {
	out := map[string]json.RawMessage{
		"code":   json.RawMessage(`20000015`),
		"result": json.RawMessage(`null`),
	}
	if got := captchaResultCode(out); got != captchaTooFrequentCode {
		t.Errorf("应识别出频控码，实际 %q", got)
	}

	// 其它码不应被误认为频控
	for _, code := range []string{`20000007`, `20000006`, `20000011`, `200`} {
		o := map[string]json.RawMessage{"code": json.RawMessage(code)}
		if captchaResultCode(o) == captchaTooFrequentCode {
			t.Errorf("code=%s 不应被识别为频控", code)
		}
	}
}

// TestCaptchaAbortMarksThrottled 校验频控错误在结果里带上可识别的 code。
func TestCaptchaAbortMarksThrottled(t *testing.T) {
	res := captchaAbortResult(ErrCaptchaThrottled, false)
	if res.Code != "captcha_throttled" {
		t.Errorf("频控应返回 captcha_throttled，实际 %q", res.Code)
	}
	if res.Success {
		t.Error("频控不应算成功")
	}
	// 提示要说明会自己退避，别让用户以为坏了
	if !strings.Contains(res.Hint, "退避") {
		t.Errorf("提示应说明自动退避，实际 %q", res.Hint)
	}

	// 用户取消不应被标成频控
	aborted := captchaAbortResult(ErrManualCaptchaAborted, true)
	if aborted.Code == "captcha_throttled" {
		t.Error("用户取消不应被标记为频控")
	}
}

// TestTenantExpired420 校验 HTTP 420 被识别为租户到期。
//
// 回归（2026-11-18 武汉轻工 whpu.campusphere.net）：所有业务接口返回
// HTTP 420 + HTML「错误 420 感谢您的使用与支持！现提醒您，产品已到期~」。
// 旧实现把它当普通 4xx，按临时失败全天重试 25 次。
func TestTenantExpired420(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(420)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>错误 420 感谢您的使用与支持！现提醒您，产品已到期~ 进入首页 退出</body></html>`))
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	_, err := c.postJSON(context.Background(), srv.URL+"/wec-counselor-attendance-apps/x", nil, nil)
	if err == nil {
		t.Fatal("420 应返回错误")
	}
	if !IsTenantExpired(err) {
		t.Fatalf("420 应被识别为租户到期，实际：%v", err)
	}
	// 不能被误判成会话失效——那会触发无意义的重登
	if IsAuthExpired(err) {
		t.Error("租户到期不应被当成会话失效")
	}
}

// TestOther4xxNotTenantExpired 校验普通 4xx 不被误判。
func TestOther4xxNotTenantExpired(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 405, 500, 502} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte("boom"))
		}))
		c := NewClient(ClientOptions{HTTPClient: srv.Client()}, nil)
		_, err := c.postJSON(context.Background(), srv.URL+"/x", nil, nil)
		srv.Close()
		if err == nil {
			continue
		}
		if IsTenantExpired(err) {
			t.Errorf("HTTP %d 不应被识别为租户到期", code)
		}
	}
}

// TestCaptchaResultCodeParsesQuotedAndRaw 校验码值兼容字符串与数字两种写法。
func TestCaptchaResultCodeParsesQuotedAndRaw(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`20000015`, "20000015"},
		{`"20000015"`, "20000015"},
		{` "20000007" `, "20000007"},
	}
	for _, c := range cases {
		o := map[string]json.RawMessage{"code": json.RawMessage(c.raw)}
		if got := captchaResultCode(o); got != c.want {
			t.Errorf("captchaResultCode(%s) = %q，期望 %q", c.raw, got, c.want)
		}
	}
}

// TestThrottleErrorWrapping 校验频控错误可被 errors.Is 穿透包装。
func TestThrottleErrorWrapping(t *testing.T) {
	wrapped := errors.Join(nil, ErrCaptchaThrottled)
	if !errors.Is(wrapped, ErrCaptchaThrottled) {
		t.Error("包装后的频控错误应能被 errors.Is 识别")
	}
}
