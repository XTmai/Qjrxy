package cpdaily

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"qjrxy/internal/vision"
)

// 端到端：第一次提交被 2210060000 拒绝 → checkValidation → create →
// 下载 9 图 → 模型给 [2,5] → validate 回填第 2、5 张的 code → 二次提交成功。
func TestSubmitRetriesOnceWhenCaptchaRequired(t *testing.T) {
	var submitCalls, checkCalls, visionCalls int32
	var gotScenesCode []string
	var gotScenesCodes []string
	var visionPrompt string

	img := capSolidPNG(t, 64, 48, color.RGBA{R: 30, G: 150, B: 120, A: 255})

	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(img)
	})
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/checkValidation", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&checkCalls, 1)
		_, _ = io.WriteString(w, `{"code":"0","datas":{"validation":true,"accountKey":"ak","sceneCode":"sc","tenantId":"1010","userId":"u1"}}`)
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("deviceId"); got != "dev-1" {
			t.Errorf("create 请求头 deviceId 应为 dev-1，实际 %q", got)
		}
		var infos []map[string]string
		for i := 1; i <= 9; i++ {
			infos = append(infos, map[string]string{
				"code": fmt.Sprintf("c%d", i),
				"path": fmt.Sprintf("%s/img/%d.png", srvURL, i),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"result": map[string]any{
				"code":       "SESSION-1",
				"name":       "自行车",
				"imageInfos": infos,
			},
		})
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/validate/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Errorf("解析 validate 表单失败：%v", err)
		}
		gotScenesCode = r.MultipartForm.Value["scenesImageCode"]
		gotScenesCodes = r.MultipartForm.Value["scenesImageCodes"]
		_, _ = io.WriteString(w, `{"code":"0","result":"TICKET-XYZ"}`)
	})
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/submitSign", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&submitCalls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		if n == 1 {
			_, _ = io.WriteString(w, `{"code":"2210060000","message":"验证码验签失败"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":"0","message":"SUCCESS"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	// 识别服务用 mock，断言 prompt 里带目标词，返回第 2、5 格
	vmux := http.NewServeMux()
	vmux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&visionCalls, 1)
		b, _ := io.ReadAll(r.Body)
		visionPrompt = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "[2,5]"}}},
		})
	})
	vsrv := httptest.NewServer(vmux)
	defer vsrv.Close()

	old := vision.Current()
	defer vision.Configure(old.APIKey, old.BaseURL, old.Model)
	vision.Configure("test-key", vsrv.URL, "deepseek-flash")

	host := strings.TrimPrefix(srv.URL, "https://")
	// 直接用 httptest 的客户端：它自带信任自签证书的 Transport，
	// 比往 TLS 白名单里塞 IP 更贴近真实调用。
	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	device := DeviceProfile{
		DeviceID: "dev-1", DeviceModel: "23127PN0CC", SystemVersion: "14",
		UserID: "u1", Lon: 118.7, Lat: 32.0,
	}
	form := map[string]any{
		"signInstanceWid": "W1", "userId": "u1",
		"lon": 118.7, "lat": 32.0, "address": "校内",
	}

	res, err := c.submit(context.Background(), host, "1010", device, KindSign, form, nil)
	if err != nil {
		t.Fatalf("提交报错：%v", err)
	}
	if !res.Success {
		t.Fatalf("二次提交应成功，实际 code=%q msg=%q hint=%q", res.Code, res.Message, res.Hint)
	}
	if got := atomic.LoadInt32(&submitCalls); got != 2 {
		t.Fatalf("提交次数应为 2，实际 %d", got)
	}
	if got := atomic.LoadInt32(&checkCalls); got != 1 {
		t.Fatalf("checkValidation 应调用 1 次，实际 %d", got)
	}
	if got := atomic.LoadInt32(&visionCalls); got != 1 {
		t.Fatalf("识别应调用 1 次（九宫格一次请求），实际 %d", got)
	}
	if len(gotScenesCode) != 1 || gotScenesCode[0] != "SESSION-1" {
		t.Fatalf("scenesImageCode 应为会话码，实际 %v", gotScenesCode)
	}
	if len(gotScenesCodes) != 2 || gotScenesCodes[0] != "c2" || gotScenesCodes[1] != "c5" {
		t.Fatalf("scenesImageCodes 应为命中图的 code [c2 c5]，实际 %v", gotScenesCodes)
	}
	if form["ticket"] != "TICKET-XYZ" {
		t.Fatalf("ticket 未回填到提交体，实际 %v", form["ticket"])
	}
	if !strings.Contains(visionPrompt, "自行车") {
		t.Fatalf("识别 prompt 未带目标词：%s", visionPrompt)
	}
	if !strings.Contains(visionPrompt, "3×3") {
		t.Fatalf("识别 prompt 未说明九宫格布局：%s", visionPrompt)
	}
}

// 不需要验证码的学校：首次提交成功，不应发出任何验证码请求。
// 这是「失败再问」方案的核心保证。
func TestSubmitSkipsCaptchaWhenAccepted(t *testing.T) {
	var submitCalls, checkCalls, createCalls int32

	mux := http.NewServeMux()
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/checkValidation", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&checkCalls, 1)
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&createCalls, 1)
	})
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/submitSign", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&submitCalls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"code":"0","message":"SUCCESS"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "https://")
	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	device := DeviceProfile{DeviceID: "dev-1", UserID: "u1", Lon: 118.7, Lat: 32.0}
	form := map[string]any{"signInstanceWid": "W1", "userId": "u1", "lon": 118.7, "lat": 32.0}

	res, err := c.submit(context.Background(), host, "1010", device, KindSign, form, nil)
	if err != nil || res == nil || !res.Success {
		t.Fatalf("提交应成功：err=%v res=%+v", err, res)
	}
	if got := atomic.LoadInt32(&submitCalls); got != 1 {
		t.Fatalf("只应提交 1 次，实际 %d", got)
	}
	if got := atomic.LoadInt32(&checkCalls); got != 0 {
		t.Fatalf("提交成功时不应询问验证码，checkValidation 实际调用 %d 次", got)
	}
	if got := atomic.LoadInt32(&createCalls); got != 0 {
		t.Fatalf("提交成功时不应创建验证码，create 实际调用 %d 次", got)
	}
	if _, ok := form["ticket"]; ok {
		t.Fatal("提交体不应出现 ticket")
	}
}

// 补了 ticket 仍被拒时，不能只把原始错误码丢给用户，要给出可操作提示。
func TestCaptchaRetryStillRejectedGivesHint(t *testing.T) {
	var submitCalls int32
	img := capSolidPNG(t, 40, 40, color.RGBA{G: 120, A: 255})

	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(img) })
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/checkValidation", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"datas":{"validation":true,"accountKey":"ak","sceneCode":"sc","tenantId":"1010","userId":"u1"}}`)
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		var infos []map[string]string
		for i := 1; i <= 9; i++ {
			infos = append(infos, map[string]string{"code": fmt.Sprintf("c%d", i), "path": fmt.Sprintf("%s/img/%d.png", srvURL, i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{
			"code": "SESSION-2", "name": "食堂", "imageInfos": infos,
		}})
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/validate/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"TICKET-2"}`)
	})
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/submitSign", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&submitCalls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"code":"2210060000","message":"验证码验签失败"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	vmux := http.NewServeMux()
	vmux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "[3]"}}},
		})
	})
	vsrv := httptest.NewServer(vmux)
	defer vsrv.Close()
	old := vision.Current()
	defer vision.Configure(old.APIKey, old.BaseURL, old.Model)
	vision.Configure("k", vsrv.URL, "deepseek-flash")

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	res, err := c.submit(context.Background(), strings.TrimPrefix(srv.URL, "https://"), "1010",
		DeviceProfile{DeviceID: "dev-1", UserID: "u1", Lon: 118.7, Lat: 32.0},
		KindSign, map[string]any{"userId": "u1", "lon": 118.7, "lat": 32.0}, nil)
	if err != nil {
		t.Fatalf("不应返回底层错误：%v", err)
	}
	if res.Success {
		t.Fatal("两次都被拒，结果应为失败")
	}
	if res.Code != captchaRequiredCode {
		t.Fatalf("应保留服务端错误码，实际 %q", res.Code)
	}
	if !strings.Contains(res.Hint, "图形验证码") {
		t.Fatalf("提示应说明验证码要求，实际 %q", res.Hint)
	}
	if got := atomic.LoadInt32(&submitCalls); got != 2 {
		t.Fatalf("只应重试一次，提交次数实际 %d", got)
	}
}
