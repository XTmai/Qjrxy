package service

import (
	"context"
	"strings"
	"testing"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/secure"
)

// TestImportPreservesSession 校验导入带凭证的账号后，cookies 与 tgc 都真正落库。
//
// 回归：导入时先写 cookies，再用整账号结构体更新 tgc，
// 而结构体里的 cookies 还是创建时的空值，导致登录态被覆盖成空。
func TestImportPreservesSession(t *testing.T) {
	svc, user := newTestService(t)
	ctx := context.Background()

	// 预置学校缓存，避免单测访问远端学校列表。
	svc.tenantMu.Lock()
	svc.tenantCache = map[string]*cpdaily.Tenant{
		"测试大学": {ID: "tenant-1", Name: "测试大学", Host: "t.campusphere.net", IDSURL: "https://t.campusphere.net/iap", JoinType: "CAS"},
	}
	svc.tenantMu.Unlock()

	res := svc.ImportAccounts(ctx, user.ID, []ExportAccount{{
		Alias:       "导入账号",
		SchoolName:  "测试大学",
		Username:    "2023002",
		DeviceID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Platform:    "android",
		LoginStatus: "authorized",
		Cookies:     map[string]string{"MOD_AUTH_CAS": "cas-token", "sessionToken": "sess-1"},
		TGC:         "TGT-abc-123",
	}})
	if res.Created != 1 || res.Failed != 0 {
		t.Fatalf("导入结果异常：%+v", res)
	}

	acc, err := svc.DB().AccountBySchoolUsername(ctx, user.ID, "测试大学", "2023002")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(acc.Cookies, "cas-token") || !strings.Contains(acc.Cookies, "sess-1") {
		t.Errorf("cookies 未落库：%q", acc.Cookies)
	}
	if acc.LoginStatus != "authorized" {
		t.Errorf("登录态应沿用导出值，得到 %q", acc.LoginStatus)
	}
	tgc, err := secure.Decrypt(svc.cfg.SecretKey, acc.TGCEnc)
	if err != nil || tgc != "TGT-abc-123" {
		t.Errorf("tgc 未正确保存：%q err=%v", tgc, err)
	}

	// 再导出一次，凭证应与导入时一致。
	file, err := svc.ExportAccounts(ctx, user.ID, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Accounts) != 1 {
		t.Fatalf("导出条数应为 1，得到 %d", len(file.Accounts))
	}
	out := file.Accounts[0]
	if out.Cookies["MOD_AUTH_CAS"] != "cas-token" || out.Cookies["sessionToken"] != "sess-1" {
		t.Errorf("导出 cookies 不一致：%+v", out.Cookies)
	}
	if out.TGC != "TGT-abc-123" {
		t.Errorf("导出 tgc 不一致：%q", out.TGC)
	}
}
