package service

import "testing"

// 短信登录只换回 MOD_AUTH_CAS，不能因为整体替换就把原有的 CAS 票据冲掉。
func TestMergeCookiesInheritsCASTGC(t *testing.T) {
	old := `{"CASTGC":"tgt-old","CONVERSATION":"conv-old","MOD_AUTH_CAS":"cas-old"}`
	fresh := map[string]string{
		"MOD_AUTH_CAS": "cas-new",
		"sessionToken": "st-new",
		"tenantId":     "tenant-1",
	}
	got := mergeCookies(old, fresh)

	if got["CASTGC"] != "tgt-old" {
		t.Errorf("CASTGC 应继承旧值，得到 %q", got["CASTGC"])
	}
	if got["CONVERSATION"] != "conv-old" {
		t.Errorf("CONVERSATION 应继承旧值，得到 %q", got["CONVERSATION"])
	}
	if got["MOD_AUTH_CAS"] != "cas-new" {
		t.Errorf("MOD_AUTH_CAS 应被新值覆盖，得到 %q", got["MOD_AUTH_CAS"])
	}
	if got["sessionToken"] != "st-new" || got["tenantId"] != "tenant-1" {
		t.Errorf("新会话的键应全部保留：%v", got)
	}
}

// 账密登录不会下发 sessionToken；旧的 sessionToken 属于上一次会话，
// 不该留在新会话里被后续请求带出去。
func TestMergeCookiesDoesNotInheritSessionToken(t *testing.T) {
	old := `{"CASTGC":"tgt","sessionToken":"stale-token"}`
	got := mergeCookies(old, map[string]string{"MOD_AUTH_CAS": "cas-new"})

	if _, ok := got["sessionToken"]; ok {
		t.Fatalf("旧 sessionToken 不应被继承：%v", got)
	}
	if got["CASTGC"] != "tgt" {
		t.Fatalf("CASTGC 应保留，得到 %q", got["CASTGC"])
	}
}

// 空值不能盖掉可用的旧值（服务端没下发 ≠ 让旧的失效）。
func TestMergeCookiesIgnoresBlankFresh(t *testing.T) {
	old := `{"CASTGC":"tgt"}`
	got := mergeCookies(old, map[string]string{"CASTGC": "   ", "MOD_AUTH_CAS": ""})

	if got["CASTGC"] != "tgt" {
		t.Fatalf("空值不应覆盖旧 CASTGC，得到 %q", got["CASTGC"])
	}
	if _, ok := got["MOD_AUTH_CAS"]; ok {
		t.Fatalf("空值不应写进结果：%v", got)
	}
}

func TestMergeCookiesEmptyOld(t *testing.T) {
	for _, old := range []string{"", "  ", "not-json"} {
		got := mergeCookies(old, map[string]string{"MOD_AUTH_CAS": "cas"})
		if got["MOD_AUTH_CAS"] != "cas" {
			t.Fatalf("旧值 %q 下应保留新会话，得到 %v", old, got)
		}
	}
}
