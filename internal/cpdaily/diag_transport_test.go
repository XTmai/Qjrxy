// 诊断覆盖率测试：锁住「所有出站请求都会被记录」这个承诺。
//
// 背景（2026-11-16）：排查安徽师范大学 NOTCLOUD 登录失败时，诊断日志里
// 只有「打开登录页」和「拉验证码图」两个 GET，没有任何 POST——因为 CAS
// 密码登录提交用的是裸 http.Client，绕过了 client.do 里的手工埋点。结果
// 只能看出「断在验证码之后」，看不出提交了什么、学校回了什么。
//
// 现在埋点下沉到 Transport 层，这组测试确保：任何新建的 http.Client（只要
// 走 HTTPClient / WrapTransport）都会落记录，且 flow / label 归到正确的桶。
package cpdaily

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"qjrxy/internal/diag"
)

// diagTestMu 让诊断相关的测试串行执行。
//
// diag 是进程级单例（一个 worker、一份 cfg），而同包测试默认并行跑。
// 两个用例各自 Configure 到不同目录时会互相劫持对方的输出，表现为
// 「只记录到 0 条」。串行是最简单可靠的隔离方式。
var diagTestMu sync.Mutex

// setupDiag 把诊断日志指向临时目录并启用，返回读取已落盘条目的函数。
//
// 诊断是异步写盘的。Close 是终态（之后无法再 Configure 启动），所以这里
// 用 Flush 同步，且不在 Cleanup 里关——同包其它用例还要继续记录。
//
// 目录用 os.MkdirTemp 而不是 t.TempDir：worker 可能还持有打开的文件，
// t.TempDir 的自动清理在 Windows 上会因文件被占用而失败。
func setupDiag(t *testing.T) func() []diag.Entry {
	t.Helper()
	diagTestMu.Lock()
	t.Cleanup(diagTestMu.Unlock)

	dir, err := os.MkdirTemp("", "qjrxy-diag-*")
	if err != nil {
		t.Fatal(err)
	}
	diag.Configure(dir, true, 1)
	t.Cleanup(func() {
		// 尽力删；删不掉也无所谓（系统临时目录），断言已经跑完。
		_ = os.RemoveAll(dir)
	})

	read := func() []diag.Entry {
		diag.Flush()
		path := filepath.Join(dir, "diag", "diag-"+time.Now().Format("20060102")+".jsonl")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var out []diag.Entry
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var e diag.Entry
			if json.Unmarshal([]byte(line), &e) == nil {
				out = append(out, e)
			}
		}
		return out
	}
	return read
}

// waitFor 轮询直到 cond 成立或超时。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待超时：%s", what)
}

// TestDiagTransportRecordsRawClientPost 校验「绕过 Client 封装、直接用
// HTTPClient 建的裸请求」也会被记录，且请求体、响应体、flow、label 都在。
//
// 这条用例直接对应本次事故：CAS 登录提交与账号体系短信都属于这种形态。
func TestDiagTransportRecordsRawClientPost(t *testing.T) {
	read := setupDiag(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"2210060000","message":"验证码验签失败"}`))
	}))
	defer srv.Close()

	ctx := WithFlow(WithLabel(context.Background(), "account:82"), FlowPasswordLogin)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/authserver/login",
		strings.NewReader("username=2024235110&password=secret"))
	if err != nil {
		t.Fatal(err)
	}
	client := HTTPClient("", 5*time.Second, nil, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	var got diag.Entry
	waitFor(t, "POST 记录落盘", func() bool {
		for _, e := range read() {
			if e.Method == http.MethodPost && strings.Contains(e.URL, "/authserver/login") {
				got = e
				return true
			}
		}
		return false
	})

	if got.Flow != FlowPasswordLogin {
		t.Errorf("flow 应为 %q，实际 %q", FlowPasswordLogin, got.Flow)
	}
	if got.Label != "account:82" {
		t.Errorf("label 应从 context 取到 account:82，实际 %q", got.Label)
	}
	if got.Status != http.StatusOK {
		t.Errorf("status 应为 200，实际 %d", got.Status)
	}
	// 响应体里的业务码是排查的关键证据，必须留下。
	if !strings.Contains(got.RespBody, "2210060000") {
		t.Errorf("响应体应包含 2210060000，实际 %q", got.RespBody)
	}
	// 密码必须脱敏，不能因为「要看请求体」而把密码写进日志。
	if strings.Contains(got.ReqBody, "secret") {
		t.Errorf("请求体里的密码未脱敏：%q", got.ReqBody)
	}
}

// TestDiagTransportMarshalNoDuplicate 校验同一次请求只产生一条记录。
//
// 手工埋点（diagStart/wrap）与 Transport 层同时存在时会写出两条，
// 让日志出现「同一次提交重复两次」的假象。
func TestDiagTransportNoDuplicateForClientDo(t *testing.T) {
	read := setupDiag(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS"}`))
	}))
	defer srv.Close()

	client := NewClient(ClientOptions{Label: "account:7"}, nil)
	if _, err := client.postJSON(context.Background(), srv.URL+"/probe", nil, nil); err != nil {
		t.Fatal(err)
	}

	// 等第一条出现，再多等一会看有没有第二条。
	waitFor(t, "第一条记录", func() bool { return len(read()) >= 1 })
	time.Sleep(200 * time.Millisecond)

	n := 0
	for _, e := range read() {
		if strings.Contains(e.URL, "/probe") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("同一次请求应只记 1 条，实际 %d 条", n)
	}
}

// TestDiagTransportDisabledPassthrough 校验诊断关闭时不记录、且请求照常。
//
// 用 SetEnabled 临时关而不是 Close：Close 是进程级终态，调用后本进程
// 再也无法记录，会把同包其它用例一起打成「只记录到 0 条」。
func TestDiagTransportDisabledPassthrough(t *testing.T) {
	diagTestMu.Lock()
	defer diagTestMu.Unlock()

	dir := t.TempDir()
	diag.Configure(dir, false, 1)
	t.Cleanup(func() { diag.SetEnabled(true) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	resp, err := HTTPClient("", 5*time.Second, nil, nil).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	path := filepath.Join(dir, "diag", "diag-"+time.Now().Format("20060102")+".jsonl")
	if _, err := os.Stat(path); err == nil {
		t.Error("诊断关闭时不应产生日志文件")
	}
}

// TestCASLoginPostsAreDiagnosed 端到端验证 CAS 账密登录的提交会被记录。
//
// 这条用例直接复刻 2026-11-16 安师大事故的形态：学校登录页要求图形验证码，
// 之前的实现在「拉完验证码图」之后就断了，而提交那一步因为用裸 http.Client
// 完全没有日志。这里确认：拿到验证码后的 POST 一定会出现在诊断里，
// 且 flow 标成 password-login、响应体能看见学校的拒绝原因。
func TestCASLoginPostsAreDiagnosed(t *testing.T) {
	read := setupDiag(t)

	var submitted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/login") && r.Method == http.MethodGet:
			// 一门要求验证码的 CAS 登录页（pwdFromId 口味）。
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body>
<form id="pwdFromId" action="/authserver/login" method="post">
  <input type="hidden" name="lt" value="LT-abc">
  <input type="hidden" name="execution" value="e1s1">
  <input type="hidden" name="_eventId" value="submit">
  <input id="pwdEncryptSalt" name="pwdEncryptSalt" value="salt1234567890ab">
  <input type="text" name="username">
  <input type="password" name="password">
  <input type="text" name="captchaResponse">
</form></body></html>`))
		case strings.Contains(r.URL.Path, "checkNeedCaptcha"):
			_, _ = w.Write([]byte("true"))
		case strings.HasSuffix(r.URL.Path, "/getCaptcha.htl"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10})
		case strings.HasSuffix(r.URL.Path, "/login") && r.Method == http.MethodPost:
			submitted = true
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// 学校以验证码错误拒绝——这正是要看得到的那种响应。
			_, _ = w.Write([]byte(`<html><body><div id="errorMsg">验证码错误或已过期</div></body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// 让 IDSURL 指向测试服务器：Tenant.LoginURL() 是 IDSURL + /login?service=...
	base := srv.URL
	tenant := &Tenant{ID: "ahnu", Name: "安徽师范大学", JoinType: "NOTCLOUD",
		Host: "ahnu.campusphere.net", IDSURL: base}
	opts := ClientOptions{Label: "account:7"}
	ctx := WithFlow(context.Background(), FlowPasswordLogin)

	ch, client, err := StartLogin(ctx, opts, tenant, "2024235110", "pw")
	if err != nil {
		t.Fatalf("打开登录页失败：%v", err)
	}
	if !ch.NeedCaptcha {
		t.Fatalf("应识别为需要验证码，实际 %+v", ch)
	}
	// 用验证码完成登录：这一步之前完全不在诊断里。
	_, _ = FinishLoginWith(ctx, opts, ch.State, "8888", client)
	if !submitted {
		t.Fatal("测试服务器没有收到提交，用例前置条件不成立")
	}

	entries := read()
	var post *diag.Entry
	for i := range entries {
		if entries[i].Method == http.MethodPost && strings.HasSuffix(strings.Split(entries[i].URL, "?")[0], "/login") {
			post = &entries[i]
			break
		}
	}
	if post == nil {
		t.Fatalf("CAS 登录提交未进诊断日志，只记录到 %d 条", len(entries))
	}
	if post.Flow != FlowPasswordLogin {
		t.Errorf("提交的 flow 应为 %q，实际 %q", FlowPasswordLogin, post.Flow)
	}
	if post.Label != "account:7" {
		t.Errorf("提交的 label 应为 account:7，实际 %q", post.Label)
	}
	if !strings.Contains(post.RespBody, "验证码错误") {
		t.Errorf("响应体应保留学校的拒绝原因，实际 %q", post.RespBody)
	}
	// 表单里的密码必须被脱敏。
	if strings.Contains(post.ReqBody, "pw=") || strings.Contains(post.ReqBody, `password=pw`) {
		t.Errorf("表单密码未脱敏：%q", post.ReqBody)
	}
	if !strings.Contains(post.ReqBody, "<redacted>") {
		t.Errorf("表单密码应替换为 <redacted>，实际 %q", post.ReqBody)
	}
}

// TestSMSLoginPostsAreDiagnosed 验证账号体系短信链路（发码 / 登录 / 补签）
// 全部进诊断日志。
//
// 这三步以前一条都不记录（appSMSPost 与 RefreshTGCBySession 各自 new 了
// 裸 http.Client），所以「短信登录不行」的反馈只能靠猜走到了哪一步。
func TestSMSLoginPostsAreDiagnosed(t *testing.T) {
	read := setupDiag(t)

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		// 外层信封：data 字段是加密串，这里给一个必然解析失败的内容，
		// 用例只关心请求是否被记录。
		_, _ = w.Write([]byte(`{"errCode":0,"data":"not-a-valid-cipher"}`))
	}))
	defer srv.Close()
	oldHost := appSMSHost
	appSMSHost = srv.URL
	defer func() { appSMSHost = oldHost }()

	ctx := WithFlow(WithLabel(context.Background(), "account:7"), FlowSMSLogin)
	_, _ = AppMobileLogin(ctx, DeviceProfile{DeviceID: "d"}, "ahnu", "13800000000", "123456", SMSChannelMob)

	if len(paths) == 0 {
		t.Fatal("测试服务器没收到请求")
	}
	entries := read()
	var got *diag.Entry
	for i := range entries {
		if strings.Contains(entries[i].URL, "mobileLogin") {
			got = &entries[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("短信登录请求未进诊断日志，只记录到 %d 条", len(entries))
	}
	if got.Flow != FlowSMSLogin {
		t.Errorf("flow 应为 %q，实际 %q", FlowSMSLogin, got.Flow)
	}
	if got.Label != "account:7" {
		t.Errorf("label 应为 account:7，实际 %q", got.Label)
	}
	if got.Method != http.MethodPost {
		t.Errorf("method 应为 POST，实际 %s", got.Method)
	}
	if got.ReqBody == "" || got.ReqBody == "<stream>" {
		t.Errorf("请求体应被记录，实际 %q", got.ReqBody)
	}
}
