package cpdaily

import (
	"context"
	"encoding/json"
	"errors"
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

// 自动识别不可用（未配 Key / 模型报错）时，把同一张九宫格交给用户点选，
// 用用户给的格子号换取 ticket，最终提交成功。
func TestSubmitFallsBackToManualWhenVisionUnavailable(t *testing.T) {
	var submitCalls, createCalls int32
	var gotScenesCodes []string
	var gotReq ManualCaptchaRequest

	img := capSolidPNG(t, 32, 32, color.RGBA{R: 40, G: 140, B: 200, A: 255})
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(img) })
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/checkValidation", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"datas":{"validation":true,"accountKey":"ak","sceneCode":"sc","tenantId":"1010","userId":"u1"}}`)
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&createCalls, 1)
		var infos []map[string]string
		for i := 1; i <= 9; i++ {
			infos = append(infos, map[string]string{
				"code": fmt.Sprintf("c%d", i),
				"path": fmt.Sprintf("%s/img/%d.png", srvURL, i),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{
			"code": "SESSION-M", "name": "自行车", "imageInfos": infos,
		}})
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/validate/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(4 << 20)
		gotScenesCodes = r.MultipartForm.Value["scenesImageCodes"]
		_, _ = io.WriteString(w, `{"result":"TICKET-MANUAL"}`)
	})
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/submitSign", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&submitCalls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"code":"0","message":"SUCCESS"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	// 关键：识别服务未配置，自动路径必然失败
	old := vision.Current()
	defer vision.Configure(old.APIKey, old.BaseURL, old.Model)
	vision.Configure("", "", "")

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	device := DeviceProfile{DeviceID: "dev-1", UserID: "u1", Lon: 118.7, Lat: 32.0}
	form := map[string]any{"userId": "u1", "lon": 118.7, "lat": 32.0}

	manual := func(ctx context.Context, req ManualCaptchaRequest) ([]int, error) {
		gotReq = req
		return []int{2, 5}, nil
	}
	res, err := c.submit(context.Background(), strings.TrimPrefix(srv.URL, "https://"), "1010",
		device, KindSign, form, manual)
	if err != nil {
		t.Fatalf("提交报错：%v", err)
	}
	if !res.Success {
		t.Fatalf("应提交成功，实际 code=%q msg=%q hint=%q", res.Code, res.Message, res.Hint)
	}
	if res.CaptchaBy != "manual" {
		t.Fatalf("应标记为人工解出，实际 %q", res.CaptchaBy)
	}
	if gotReq.Name != "自行车" {
		t.Fatalf("目标词应透传给用户，实际 %q", gotReq.Name)
	}
	if gotReq.Cells != 9 {
		t.Fatalf("格子数应为 9，实际 %d", gotReq.Cells)
	}
	if !strings.HasPrefix(gotReq.Grid, "data:image/jpeg;base64,") {
		t.Fatalf("九宫格应以 data URL 下发，实际前缀 %q", truncateStr(gotReq.Grid, 40))
	}
	if len(gotScenesCodes) != 2 || gotScenesCodes[0] != "c2" || gotScenesCodes[1] != "c5" {
		t.Fatalf("人工选中的格子应映射回 code [c2 c5]，实际 %v", gotScenesCodes)
	}
	if got := atomic.LoadInt32(&createCalls); got != 1 {
		t.Fatalf("人工一次点选只应 create 一次，实际 %d", got)
	}
	if got := atomic.LoadInt32(&submitCalls); got != 1 {
		t.Fatalf("先解验证码再提交，提交次数应为 1，实际 %d", got)
	}
}

// 用户点「取消」后立即结束，不再弹第二次，也不再提交。
//
// 新流程下 checkValidation 探路就已经知道要验证码，用户取消时还
// 一次提交都没发出——比旧流程更干净，不会在学校侧留下失败记录。
func TestSubmitManualCancelAbortsWithoutRetry(t *testing.T) {
	var submitCalls, createCalls int32
	img := capSolidPNG(t, 24, 24, color.RGBA{B: 200, A: 255})

	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(img) })
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/checkValidation", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"datas":{"validation":true,"accountKey":"ak","sceneCode":"sc","tenantId":"1010","userId":"u1"}}`)
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&createCalls, 1)
		var infos []map[string]string
		for i := 1; i <= 9; i++ {
			infos = append(infos, map[string]string{"code": fmt.Sprintf("c%d", i), "path": fmt.Sprintf("%s/img/%d.png", srvURL, i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{
			"code": "SESSION-C", "name": "食堂", "imageInfos": infos,
		}})
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/validate/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		t.Error("用户取消后不应再调用 validate")
	})
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/submitSign", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&submitCalls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"code":"2210060000","message":"验证码验签失败"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := vision.Current()
	defer vision.Configure(old.APIKey, old.BaseURL, old.Model)
	vision.Configure("", "", "")

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	manual := func(ctx context.Context, req ManualCaptchaRequest) ([]int, error) {
		return nil, AbortManualCaptcha("已取消本次验证码")
	}
	res, err := c.submit(context.Background(), strings.TrimPrefix(srv.URL, "https://"), "1010",
		DeviceProfile{DeviceID: "dev-1", UserID: "u1", Lon: 118.7, Lat: 32.0},
		KindSign, map[string]any{"userId": "u1"}, manual)
	if err != nil {
		t.Fatalf("取消不应变成底层错误：%v", err)
	}
	if res.Success {
		t.Fatal("取消后不应报告成功")
	}
	if !strings.Contains(res.Message, "取消") {
		t.Fatalf("应说明是用户取消，实际 msg=%q hint=%q", res.Message, res.Hint)
	}
	if got := atomic.LoadInt32(&submitCalls); got != 0 {
		t.Fatalf("取消后不应提交，实际 %d 次", got)
	}
	if got := atomic.LoadInt32(&createCalls); got != 1 {
		t.Fatalf("取消后不应刷新验证码，create 实际 %d 次", got)
	}
}

// 用户点「换一张」时重新 create，第二次点选后成功。
func TestSubmitManualSkipRefreshesChallenge(t *testing.T) {
	var createCalls, manualCalls int32

	img := capSolidPNG(t, 24, 24, color.RGBA{G: 180, A: 255})
	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(img) })
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/checkValidation", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"datas":{"validation":true,"accountKey":"ak","sceneCode":"sc","tenantId":"1010","userId":"u1"}}`)
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/create/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&createCalls, 1)
		var infos []map[string]string
		for i := 1; i <= 9; i++ {
			infos = append(infos, map[string]string{"code": fmt.Sprintf("c%d", i), "path": fmt.Sprintf("%s/img/%d.png", srvURL, i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{
			"code": fmt.Sprintf("SESSION-%d", n), "name": "图书馆", "imageInfos": infos,
		}})
	})
	mux.HandleFunc("/captcha-open-api/v1/captcha/validate/scenesImage", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"TICKET-S"}`)
	})
	var submitCalls int32
	mux.HandleFunc("/wec-counselor-sign-apps/stu/sign/submitSign", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&submitCalls, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, `{"code":"0","message":"SUCCESS"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	old := vision.Current()
	defer vision.Configure(old.APIKey, old.BaseURL, old.Model)
	vision.Configure("", "", "")

	// 把两种重试等待都压到 1ms，测试不该真的睡过去。
	oldWait := captchaRetryWait
	oldRecognize := captchaRecognizeRetryWait
	captchaRetryWait = time.Millisecond
	captchaRecognizeRetryWait = time.Millisecond
	defer func() {
		captchaRetryWait = oldWait
		captchaRecognizeRetryWait = oldRecognize
	}()

	hc := srv.Client()
	hc.Timeout = 5 * time.Second
	c := NewClient(ClientOptions{HTTPClient: hc}, nil)
	manual := func(ctx context.Context, req ManualCaptchaRequest) ([]int, error) {
		if atomic.AddInt32(&manualCalls, 1) == 1 {
			return nil, ErrManualCaptchaSkip
		}
		return []int{1}, nil
	}
	res, err := c.submit(context.Background(), strings.TrimPrefix(srv.URL, "https://"), "1010",
		DeviceProfile{DeviceID: "dev-1", UserID: "u1", Lon: 118.7, Lat: 32.0},
		KindSign, map[string]any{"userId": "u1"}, manual)
	if err != nil {
		t.Fatalf("提交流程报错：%v", err)
	}
	if !res.Success {
		t.Fatalf("换一张后应成功，实际 code=%q msg=%q", res.Code, res.Message)
	}
	if got := atomic.LoadInt32(&createCalls); got != 2 {
		t.Fatalf("换一张应重新 create，实际 %d 次", got)
	}
	if got := atomic.LoadInt32(&manualCalls); got != 2 {
		t.Fatalf("用户应被问两次，实际 %d 次", got)
	}
}

// 保证 ErrManualCaptchaSkip 能被 errors.Is 识别（避免被包装后丢失语义）。
func TestManualSkipErrorIsSentinel(t *testing.T) {
	wrapped := fmt.Errorf("外层包装：%w", ErrManualCaptchaSkip)
	if !errors.Is(wrapped, ErrManualCaptchaSkip) {
		t.Fatal("换一张的哨兵错误应能被 errors.Is 命中")
	}
}
