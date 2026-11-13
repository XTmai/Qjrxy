package cpdaily

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 测试向量由独立实现（Python/AES-CBC）生成，输入均为虚构手机号。
func TestAppSMSEncryptVector(t *testing.T) {
	got, err := appSMSEncrypt("13800138000")
	if err != nil {
		t.Fatal(err)
	}
	if want := "WNY5WA9AjqvNAR+VXK57KQ=="; got != want {
		t.Fatalf("加密结果不符：got=%s want=%s", got, want)
	}
}

func TestAppSMSEncryptLoginBody(t *testing.T) {
	body := `{"d":"123456","e":"CPDAILY","c":"13800138000","f":"86"}`
	got, err := appSMSEncrypt(body)
	if err != nil {
		t.Fatal(err)
	}
	want := "H02WNylvathW56oyb6Fu/SyfXeHgcdH5GRKo3l8VD3gyBKH8TfQQYZJG5KJ8caf5KhQ4XKzE10BqOnsTJknNwg=="
	if got != want {
		t.Fatalf("加密结果不符：got=%s want=%s", got, want)
	}
}

func TestAppSMSRoundTrip(t *testing.T) {
	for _, in := range []string{"13800138000", `{"d":"000000","e":"CPDAILY","c":"13800138000","f":"86"}`} {
		enc, err := appSMSEncrypt(in)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := appSMSDecrypt(enc)
		if err != nil {
			t.Fatal(err)
		}
		if string(dec) != in {
			t.Fatalf("往返不一致：got=%q want=%q", dec, in)
		}
	}
}

func TestAppLoginResultParse(t *testing.T) {
	// 模拟登录响应解密后的 JSON（字段名与真实响应一致，值虚构）
	raw := `{"authStatus":"auth","openId":"2024000001","sessionToken":"session-token-x",` +
		`"status":"cpdailyPage","tenantId":"tenant-x","tenantName":"示例大学",` +
		`"tgc":"iap-10000000000-TGT-0000","tgcStatus":"tgcValidate"}`
	var res AppLoginResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatal(err)
	}
	if res.TGC == "" || res.SessionToken == "" || res.OpenID != "2024000001" {
		t.Fatalf("字段解析异常：%+v", res)
	}
}

func TestIAPLoginURLCarriesService(t *testing.T) {
	raw := iapLoginURL("nut.campusphere.net")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if got, want := u.Path, "/iap/login"; got != want {
		t.Fatalf("路径应为 %s，得到 %s", want, got)
	}
	// CAS 需要 service 才知道把票据签给哪个业务域，缺了它换票永远失败。
	if got, want := u.Query().Get("service"), "https://nut.campusphere.net/portal/login"; got != want {
		t.Fatalf("service 应为 %q，得到 %q", want, got)
	}
}

// TestRefreshTGCBySession 校验 sessionToken 换取 CAS 票的请求与解析。
func TestRefreshTGCBySession(t *testing.T) {
	var gotPath, gotCookie, gotSessionKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCookie = r.Header.Get("Cookie")
		gotSessionKey = r.Header.Get("sessionTokenKey")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errCode":0,"errMsg":null,"data":{"tgc":"iap-1-TGT-new","sessionToken":"session-new","openId":"2024000001","tgcStatus":"tgcValidate"}}`))
	}))
	defer srv.Close()

	oldHost := appSMSHost
	appSMSHost = srv.URL
	defer func() { appSMSHost = oldHost }()

	res, err := RefreshTGCBySession(context.Background(), DeviceProfile{DeviceID: "device-x"}, "tenant-x", "session-old", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.TGC != "iap-1-TGT-new" || res.SessionToken != "session-new" || res.OpenID != "2024000001" {
		t.Fatalf("解析异常：%+v", res)
	}
	if gotPath != "/v6/auth/authentication/new/validation/v-9012" {
		t.Errorf("请求路径异常：%s", gotPath)
	}
	if !strings.Contains(gotCookie, "sessionToken=session-old") {
		t.Errorf("Cookie 未带 sessionToken：%s", gotCookie)
	}
	if gotSessionKey != "session-old" {
		t.Errorf("sessionTokenKey 头异常：%s", gotSessionKey)
	}
	if !strings.Contains(gotBody, `"tgc":""`) || !strings.Contains(gotBody, `"idsToken":""`) {
		t.Errorf("请求体异常：%s", gotBody)
	}
}
