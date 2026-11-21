// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

// 用 2026-11-18 生产 diag 的真实响应样本做端到端回放。
//
// 这个测试的目的不是覆盖代码分支，而是回答一个问题：
// 「把今天的真实故障场景喂给修复后的代码，行为是否真的变了？」
// 每个用例都复刻当天的完整调用序列，断言修复后的关键差异。

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// replayReplay 复刻 2026-11-18 安徽城市管理职业学院 25231011 的完整场景。
//
// 当天真实序列（21:16:30 起，7 分钟内 23 轮）：
//
//	详情 → signMode=5「每日NFC签到」
//	→ checkValidation validation=true
//	→ create 九宫格 + 下载 9 张图 + validate 换 ticket
//	→ submitSign → 2210060000「请前往nfc机器位置签到！」
//	→ 因为是待核对，下一轮再来一遍
//
// 修复后应有的行为：详情拿到 signMode=5 就停，一个验证码请求都不发。
func TestReplayNFCAccount20261118(t *testing.T) {
	var captchaCreate, captchaValidate, imageDownload, submit int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.Contains(p, "detailSignInstance"):
			// 当天详情的真实字段（截取关键部分）
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","datas":{` +
				`"signInstanceWid":"46993","signMode":5,"signRate":"1","signCondition":0,` +
				`"taskName":"每日NFC签到","senderUserName":"杨智(城市更新学院)",` +
				`"signStatus":"0","currentTime":"2026-11-18 21:16:30",` +
				`"singleTaskBeginTime":"2026-11-18 21:00","singleTaskEndTime":"2026-11-18 23:59"}}`))
		case strings.Contains(p, "checkValidation"):
			// 当天真实响应：validation=true
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","datas":{` +
				`"validation":true,"accountKey":"62de2b14e26afe6487b2587e",` +
				`"sceneCode":"62de32e3e26ac02f3d73f94b",` +
				`"tenantId":"1018764144934965","userId":"25231011"}}`))
		case strings.Contains(p, "captcha/create"):
			atomic.AddInt32(&captchaCreate, 1)
			_, _ = w.Write([]byte(`{"code":200,"message":"操作成功","result":{` +
				`"code":"401dfaa5a4204423b1b6b457f3ffe067","name":"满天星","imageInfos":[` +
				`{"code":"a","path":"https://wecloud-captcha.campusphere.net/captcha/x/1.jpg"}]}}`))
		case strings.Contains(p, "captcha/validate"):
			atomic.AddInt32(&captchaValidate, 1)
			_, _ = w.Write([]byte(`{"code":200,"message":"操作成功","result":"9a082157ed184fc694db7afd20fff554"}`))
		case strings.Contains(p, "usecloud-captcha") || strings.Contains(r.URL.Host, "wecloud-captcha"):
			atomic.AddInt32(&imageDownload, 1)
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(p, "submitSign"):
			atomic.AddInt32(&submit, 1)
			_, _ = w.Write([]byte(`{"code":"2210060000","message":"当前查寝任务不支持手机签到，请前往nfc机器位置签到！","userId":null,"userName":null}`))
		default:
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","datas":{}}`))
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)

	res, detail, err := c.Sign(context.Background(), stripScheme(srv.URL), "1018764144934965",
		DeviceProfile{DeviceID: "dev-7d8f", UserID: "25231011", Lon: 117.2, Lat: 31.8},
		SignOptions{
			Kind:     KindAttendance,
			Task:     Task{SignInstanceWID: "46993", SignWID: "200585087", TaskName: "每日NFC签到"},
			Lon:      117.2,
			Lat:      31.8,
			Position: "宿舍",
		})
	if err != nil {
		t.Fatalf("不应返回底层错误：%v", err)
	}
	if detail == nil {
		t.Fatal("应返回详情（用于审计）")
	}
	if detail.SignMode.Int() != 5 {
		t.Errorf("详情里的 signMode 应为 5，实际 %d", detail.SignMode.Int())
	}

	// 核心断言：修复后一个验证码请求都不该发
	if got := atomic.LoadInt32(&captchaCreate); got != 0 {
		t.Errorf("NFC 任务不应创建验证码会话，实际 %d 次", got)
	}
	if got := atomic.LoadInt32(&captchaValidate); got != 0 {
		t.Errorf("NFC 任务不应换取 ticket，实际 %d 次", got)
	}
	if got := atomic.LoadInt32(&submit); got != 0 {
		t.Errorf("NFC 任务不应提交，实际 %d 次", got)
	}

	// 结果必须是「确定性不支持」，而不是含混的失败
	if res.Success {
		t.Error("不应标记为成功")
	}
	if res.Code != "sign_mode_nfc" {
		t.Errorf("应返回 sign_mode_nfc，实际 %q", res.Code)
	}
	if !strings.Contains(res.Hint, "NFC") || !strings.Contains(res.Hint, "计划") {
		t.Errorf("提示应说明是 NFC 任务并指引排除计划，实际 %q", res.Hint)
	}
}

// TestReplayTenantExpired420 复刻武汉轻工当天的 420 场景。
func TestReplayTenantExpired420(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(420)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"/></head>` +
			`<body>错误 420 感谢您的使用与支持！现提醒您，产品已到期~ 进入首页 退出</body></html>`))
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{HTTPClient: srv.Client()}, nil)
	_, err := c.postJSON(context.Background(), srv.URL+
		"/wec-counselor-attendance-apps/student/attendance/getStuAttendacesInOneDay", nil, nil)
	if err == nil {
		t.Fatal("420 应报错")
	}
	if !IsTenantExpired(err) {
		t.Fatalf("应识别为租户到期：%v", err)
	}
	// 绝对不能同时被当成会话失效，否则会触发无意义的重登
	if IsAuthExpired(err) {
		t.Error("租户到期不应触发会话恢复流程")
	}
}

// TestReplayCaptchaThrottle20261118 复刻当天 validate 返回 20000015 的场景。
//
// 断言两点：
//  1. 频控错误被单独标记（ErrCaptchaThrottled），而不是混成普通识别失败；
//  2. 命中后本轮立即结束，不会在限流窗口内再发 create。
func TestReplayCaptchaThrottle20261118(t *testing.T) {
	var createCalls int32
	mux := http.NewServeMux()
	// 九宫格图片必须是可解码的真图，否则流程会在下载阶段就失败，
	// 走不到 validate——这正是第一次写这个测试时踩到的坑。
	var pngOnce sync.Once
	var pngBytes []byte
	mux.HandleFunc("/img.png", func(w http.ResponseWriter, r *http.Request) {
		pngOnce.Do(func() {
			img := image.NewRGBA(image.Rect(0, 0, 8, 8))
			var buf bytes.Buffer
			_ = png.Encode(&buf, img)
			pngBytes = buf.Bytes()
		})
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	})
	var baseURL string
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&createCalls, 1)
		// 9 张候选图，指向本测试服务
		var infos []string
		for i := 0; i < 9; i++ {
			infos = append(infos, `{"code":"c`+string(rune('a'+i))+`","path":"`+baseURL+`/img.png"}`)
		}
		_, _ = w.Write([]byte(`{"code":200,"message":"操作成功","result":{` +
			`"code":"sess-1","name":"西瓜","imageInfos":[` + strings.Join(infos, ",") + `]}}`))
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/validate/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		// 当天真实响应
		_, _ = w.Write([]byte(`{"code":20000015,"message":"验证过于频繁，请稍后再试!","result":null}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	// 把重试等待压到毫秒级，避免测试真的等
	oldRecognize, oldRetry := captchaRecognizeRetryWait, captchaRetryWait
	captchaRecognizeRetryWait = time.Millisecond
	captchaRetryWait = time.Millisecond
	defer func() {
		captchaRecognizeRetryWait, captchaRetryWait = oldRecognize, oldRetry
	}()

	c := NewClient(ClientOptions{HTTPClient: srv.Client()}, nil)
	ch := &CaptchaChallenge{
		Required: true, AccountKey: "62de2b14e26afe6487b2587e",
		SceneCode: "62de32e3e26ac02f3d73f94b",
		TenantID:  "1018764144934965", UserID: "25231011",
	}
	// 提供识别结果，跳过模型调用（本用例只关心 validate 被频控后的行为）
	manual := func(ctx context.Context, req ManualCaptchaRequest) ([]int, error) {
		return []int{1}, nil
	}
	// 让 pickByVision 走人工兜底：vision 未配置时自动识别必然返回错误
	_, _, err := c.solveCaptcha(context.Background(), stripScheme(srv.URL), ch, "dev-1", manual)
	if err == nil {
		t.Fatal("频控应返回错误")
	}
	if !strings.Contains(err.Error(), "过于频繁") {
		t.Errorf("错误应包含服务端原文，实际 %v", err)
	}
	// 频控后应立即结束本轮，只发一次 create（不再换新图重试）
	if got := atomic.LoadInt32(&createCalls); got != 1 {
		t.Errorf("命中频控后不应再换图重试，create 应只调 1 次，实际 %d 次", got)
	}
}
