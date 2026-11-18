// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package diag

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 记录 → 落盘 → 脱敏 → 按天文件，整条链路一次验证。
func TestRecordWritesSanitizedJSONL(t *testing.T) {
	dir := t.TempDir()
	Configure(dir, true, 7)
	defer Close()

	Record(Entry{
		Label:  "account:22",
		Method: http.MethodPost,
		URL:    "https://nut.campusphere.net/wec-counselor-attendance-apps/student/attendance/checkValidation",
		ReqHeaders: SanitizeHeaders(http.Header{
			"Cookie":       {"MOD_AUTH_CAS=ST-iap-secret-value-abcdefghijklmnop; sessionToken=uuid-123"},
			"CpdailyInfo":  {"ZW5jcnlwdGVk"},
			"Content-Type": {"application/json"},
			"X-Custom":     {"visible"},
		}),
		ReqBody:  SanitizeBody([]byte(`{"deviceId":"dev-1","password":"secret123"}`), 8<<10),
		RespBody: `{"code":"0","message":"SUCCESS"}`,
		Status:   200,
		Ms:       42,
	})
	// 等异步 worker 落盘
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f, err := os.Open(filepath.Join(dir, "diag", "diag-"+time.Now().Format("20060102")+".jsonl")); err == nil {
			f.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	Close() // 再 flush 一次兜底

	f, err := os.Open(filepath.Join(dir, "diag", "diag-"+time.Now().Format("20060102")+".jsonl"))
	if err != nil {
		t.Fatalf("诊断文件未生成：%v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var lines []map[string]any
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("行不是合法 JSON：%v", err)
		}
		lines = append(lines, m)
	}
	if len(lines) != 1 {
		t.Fatalf("应有 1 条记录，得到 %d", len(lines))
	}
	e := lines[0]
	// Cookie 已脱敏：只有前 6 位与长度
	hdr, _ := e["reqHeaders"].(map[string]any)
	cookie, _ := hdr["Cookie"].(string)
	if strings.Contains(cookie, "secret-value-abcdef") || !strings.Contains(cookie, "(len") {
		t.Fatalf("Cookie 未正确脱敏：%q", cookie)
	}
	if hdr["CpdailyInfo"] != "<set>" {
		t.Fatalf("凭证头应记 <set>：%q", hdr["CpdailyInfo"])
	}
	if hdr["X-Custom"] != "visible" {
		t.Fatalf("普通头应保留：%q", hdr["X-Custom"])
	}
	// 密码已替换
	if strings.Contains(e["reqBody"].(string), "secret123") {
		t.Fatalf("密码未脱敏：%q", e["reqBody"])
	}
	if !strings.Contains(e["reqBody"].(string), "redacted") {
		t.Fatalf("密码应替换为 <redacted>：%q", e["reqBody"])
	}
	if e["label"] != "account:22" || e["status"] != float64(200) {
		t.Fatalf("label/status 不符：%v", e)
	}
}

// 关闭开关后不写任何内容。
func TestDisabledWritesNothing(t *testing.T) {
	dir := t.TempDir()
	Configure(dir, false, 7)
	defer Close()
	Record(Entry{Method: http.MethodGet, URL: "https://example.com"})
	time.Sleep(100 * time.Millisecond)
	Close()
	if _, err := os.Stat(filepath.Join(dir, "diag")); !os.IsNotExist(err) {
		// 目录可能因其他用例存在，这里只断言没有当日文件
		if f, err := os.Stat(filepath.Join(dir, "diag", "diag-"+time.Now().Format("20060102")+".jsonl")); err == nil && f.Size() > 0 {
			t.Fatalf("关闭状态下不应产生记录")
		}
	}
}
