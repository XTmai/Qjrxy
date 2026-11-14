package cpdaily

import "testing"

// 用户手填的 host 形态很杂，带 scheme 会让换票地址拼成 https://https://…
// 学校域名必须归一化成纯 host[:port]。
func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"nut.campusphere.net":                     "nut.campusphere.net",
		"  nut.campusphere.net  ":                 "nut.campusphere.net",
		"https://nut.campusphere.net":             "nut.campusphere.net",
		"http://nut.campusphere.net/portal/login": "nut.campusphere.net",
		"nut.campusphere.net/portal/login":        "nut.campusphere.net",
		"https://nut.campusphere.net:8443/iap":    "nut.campusphere.net:8443",
		"":                                        "",
		"   ":                                     "",
		"https://":                                "",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// campusphere/cpdaily 域名优先于学校自己的门户域名：只有前者才带
// /iap/login 与 /wec-counselor-* 接口。
func TestHostFromEntryPrefersCampus(t *testing.T) {
	entry := &tenantEntry{
		AmpURL:  "https://portal.example.edu.cn/app",
		AmpURL2: "https://nut.campusphere.net/wec-amp/index.html",
	}
	if got, want := hostFromEntry(entry), "nut.campusphere.net"; got != want {
		t.Fatalf("hostFromEntry = %q，期望 %q", got, want)
	}
}

func TestHostFromEntryFallsBackToFirst(t *testing.T) {
	entry := &tenantEntry{AmpURL: "https://portal.example.edu.cn/app"}
	if got, want := hostFromEntry(entry), "portal.example.edu.cn"; got != want {
		t.Fatalf("hostFromEntry = %q，期望 %q", got, want)
	}
}

func TestHostFromEntryEmpty(t *testing.T) {
	if got := hostFromEntry(&tenantEntry{}); got != "" {
		t.Fatalf("空入口应返回空 host，得到 %q", got)
	}
}

// 没有租户 ID 时不该发请求，直接报错。
func TestResolveTenantByIDRequiresID(t *testing.T) {
	if _, err := ResolveTenantByID(nil, "  "); err == nil {
		t.Fatal("空租户 ID 应报错")
	}
}

// TestHostFromEntryPrefersCounselorPortal 学校自建部署的辅导猫门户
// （自有域名 + /wec-portal-mobile/ 路径）优先于学校的 ehall：
// ampUrl 常被填成 ehall（办事大厅），上面没有辅导猫接口。
// 回归：厦门医学院（ehall.xmmc.edu.cn + cat.xmmc.edu.cn/wec-portal-mobile）
// 旧逻辑回退到 ehall，业务接口全部 404。
func TestHostFromEntryPrefersCounselorPortal(t *testing.T) {
	entry := &tenantEntry{
		AmpURL:  "https://ehall.xmmc.edu.cn/newmobile/client",
		AmpURL2: "https://cat.xmmc.edu.cn/wec-portal-mobile/client",
	}
	if got, want := hostFromEntry(entry), "cat.xmmc.edu.cn"; got != want {
		t.Fatalf("hostFromEntry = %q，期望 %q", got, want)
	}
}

// TestHostFromEntryCounselorPortalOnCampusphere 托管域优先级仍最高：
// ampUrl2 同样带 /wec-portal-mobile/，但含 campusphere 时应选 campusphere。
// 实例：安徽城市管理职业学院（ehall.cua.edu.cn + cmoc.campusphere.net）。
func TestHostFromEntryCounselorPortalOnCampusphere(t *testing.T) {
	entry := &tenantEntry{
		AmpURL:  "https://ehall.cua.edu.cn/newmobile/client",
		AmpURL2: "https://cmoc.campusphere.net/wec-portal-mobile/client",
	}
	if got, want := hostFromEntry(entry), "cmoc.campusphere.net"; got != want {
		t.Fatalf("hostFromEntry = %q，期望 %q", got, want)
	}
}
