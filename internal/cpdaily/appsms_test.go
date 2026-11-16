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

// TestEnsureAppLoginTGCForNotCloudSchools 覆盖首次短信登录只给 sessionToken 的学校。
// 安徽师范大学与阜阳职业技术学院均属于 NOTCLOUD，这里使用其真实租户 ID，
// 验证两校都会立即走 validation 补签，并使用补签后的新票据和会话。
func TestEnsureAppLoginTGCForNotCloudSchools(t *testing.T) {
	cases := []struct {
		name     string
		tenantID string
	}{
		{"安徽师范大学", "ahnu"},
		{"阜阳职业技术学院", "41ad1a67-80f9-4628-873a-03cef454d41f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v6/auth/authentication/new/validation/v-9012" {
					t.Errorf("补签路径异常：%s", r.URL.Path)
				}
				if got := r.Header.Get("tenantId"); got != tc.tenantID {
					t.Errorf("tenantId 头异常：%s", got)
				}
				if !strings.Contains(r.Header.Get("Cookie"), "sessionToken=session-first") {
					t.Errorf("补签未带首次登录会话：%s", r.Header.Get("Cookie"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"errCode":0,"data":{"tgc":"iap-1-TGT-new","sessionToken":"session-refreshed","status":"cpdailyPage","tgcStatus":"tgcValidate"}}`))
			}))
			defer srv.Close()
			oldHost := appSMSHost
			appSMSHost = srv.URL
			defer func() { appSMSHost = oldHost }()

			result := &AppLoginResult{SessionToken: "session-first", TenantID: tc.tenantID}
			if err := EnsureAppLoginTGC(context.Background(), DeviceProfile{DeviceID: "device-x"}, tc.tenantID, result); err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("缺失 TGC 时应补签一次，实际 %d 次", requests)
			}
			if result.TGC != "iap-1-TGT-new" || result.SessionToken != "session-refreshed" {
				t.Fatalf("补签结果未合并：%+v", result)
			}
			if result.TenantID != tc.tenantID {
				t.Fatalf("补签不应覆盖学校租户：%s", result.TenantID)
			}
		})
	}
}

func TestEnsureAppLoginTGCSkipsUnnecessaryRefresh(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errCode":0,"data":{"tgc":"should-not-be-used"}}`))
	}))
	defer srv.Close()
	oldHost := appSMSHost
	appSMSHost = srv.URL
	defer func() { appSMSHost = oldHost }()

	withTGC := &AppLoginResult{TGC: "tgc-existing", SessionToken: "session-old"}
	if err := EnsureAppLoginTGC(context.Background(), DeviceProfile{}, "ahnu", withTGC); err != nil {
		t.Fatal(err)
	}
	withoutSession := &AppLoginResult{}
	if err := EnsureAppLoginTGC(context.Background(), DeviceProfile{}, "ahnu", withoutSession); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("已有 TGC 或没有 sessionToken 时不应请求，实际 %d 次", requests)
	}
	if withTGC.TGC != "tgc-existing" || withTGC.SessionToken != "session-old" {
		t.Fatalf("已有凭证不应被改写：%+v", withTGC)
	}
}

func TestEnsureAppLoginTGCStillMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errCode":0,"data":{"sessionToken":"session-refreshed","status":"cpdailyPage"}}`))
	}))
	defer srv.Close()
	oldHost := appSMSHost
	appSMSHost = srv.URL
	defer func() { appSMSHost = oldHost }()

	result := &AppLoginResult{SessionToken: "session-first"}
	err := EnsureAppLoginTGC(context.Background(), DeviceProfile{}, "ahnu", result)
	if err == nil || !strings.Contains(err.Error(), "仍未返回 CAS 票") {
		t.Fatalf("补签后仍无 TGC 应明确报错，得到 %v", err)
	}
	if result.SessionToken != "session-refreshed" {
		t.Fatalf("即使未补到 TGC，也应保留服务端轮换的新会话：%+v", result)
	}
}

// TestFreshMODAuthCAS 校验旧票不能算换票成功，只有新值才算。
func TestFreshMODAuthCAS(t *testing.T) {
	if freshMODAuthCAS("old", map[string]string{"MOD_AUTH_CAS": "old"}) {
		t.Fatal("旧票不应算作新会话")
	}
	if !freshMODAuthCAS("old", map[string]string{"MOD_AUTH_CAS": "new"}) {
		t.Fatal("新票应算作换票成功")
	}
	if !freshMODAuthCAS("", map[string]string{"MOD_AUTH_CAS": "new"}) {
		t.Fatal("进入前无票时应算成功")
	}
}
