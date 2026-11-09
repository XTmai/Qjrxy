package cpdaily

import (
	"encoding/json"
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
