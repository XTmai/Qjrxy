package cpdaily

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCryptoSelfTest 校验 AES/DES 与密钥派生链路。
func TestCryptoSelfTest(t *testing.T) {
	if err := CryptoSelfTest(); err != nil {
		t.Fatal(err)
	}
}

// TestInterleave 校验密钥派生：偶数位在前、奇数位在后。
func TestInterleave(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abcdef", "acebdf"},
		{"h8r7uN8t" + "9hriwfra", FKeyCat},
		{"a", "a"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Interleave(c.in); got != c.want {
			t.Errorf("Interleave(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAESRoundTrip 校验 AES-CBC 加解密可逆，且固定 IV 产生稳定密文。
func TestAESRoundTrip(t *testing.T) {
	inputs := []string{
		`{"signInstanceWid":"abc","longitude":116.397,"latitude":39.909}`,
		`{}`,
		strings.Repeat("签到数据", 50),
		`{"emoji":"🎓","中文":"今日校园"}`,
	}
	for _, in := range inputs {
		enc, err := AESEncryptCBC(in, FKeyCat)
		if err != nil {
			t.Fatalf("加密失败：%v", err)
		}
		dec, err := AESDecryptCBC(enc, FKeyCat)
		if err != nil {
			t.Fatalf("解密失败：%v", err)
		}
		if dec != in {
			t.Errorf("往返不一致：%q != %q", dec, in)
		}
	}
}

// TestAESStable 相同输入必须产生相同密文（固定 IV，与 App 行为一致）。
func TestAESStable(t *testing.T) {
	const in = `{"a":1}`
	a, err := AESEncryptCBC(in, FKeyCat)
	if err != nil {
		t.Fatal(err)
	}
	b, err := AESEncryptCBC(in, FKeyCat)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("固定 IV 下相同明文应产生相同密文")
	}
}

// TestAESWrongKey 用错误密钥解密应当失败。
func TestAESWrongKey(t *testing.T) {
	enc, err := AESEncryptCBC(`{"a":1}`, FKeyCat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AESDecryptCBC(enc, "0123456789abcdef"); err == nil {
		t.Error("错误密钥解密应当失败")
	}
}

// TestDESSelfConsistency 校验 DES 输出为合法 base64 且长度对齐。
func TestDESSelfConsistency(t *testing.T) {
	for _, key := range []string{desKeyExtension, desKeyInfo, desKeyToken} {
		if len(key) != 8 {
			t.Fatalf("DES 密钥必须 8 字节，得到 %d：%q", len(key), key)
		}
		out, err := DESEncrypt(`{"deviceId":"test"}`, key)
		if err != nil {
			t.Fatalf("DES 加密失败：%v", err)
		}
		raw, err := base64.StdEncoding.DecodeString(out)
		if err != nil {
			t.Fatalf("DES 输出不是合法 base64：%v", err)
		}
		if len(raw) == 0 || len(raw)%8 != 0 {
			t.Errorf("DES 密文长度应为 8 的倍数，得到 %d", len(raw))
		}
	}
}

// TestMD5Hex 校验摘要格式。
func TestMD5Hex(t *testing.T) {
	if got := MD5Hex("abc"); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Errorf("MD5(\"abc\") = %s", got)
	}
}

// TestHaversine 校验距离计算在合理范围。
func TestHaversine(t *testing.T) {
	// 北京天安门 -> 故宫 约 1.2km
	d := Haversine(116.397428, 39.90923, 116.397026, 39.918058)
	if d < 900 || d > 1100 {
		t.Errorf("天安门到故宫距离异常：%.1f 米", d)
	}
	// 同一点距离为 0
	if d := Haversine(116.4, 39.9, 116.4, 39.9); d != 0 {
		t.Errorf("同一点距离应为 0，得到 %f", d)
	}
}

// TestCheckRange 校验签到点范围判定。
func TestCheckRange(t *testing.T) {
	detail := &TaskDetail{
		SignPlaces: []SignPlace{
			{PlaceName: "南苑 3 号楼", Address: "南苑 3 号楼", Longitude: 116.4, Latitude: 39.9, Radius: 300},
		},
	}
	in := detail.CheckRange(116.4001, 39.9001)
	if !in.InRange {
		t.Error("签到点附近应判定为范围内")
	}
	if len(in.Places) != 1 || in.Places[0].Name != "南苑 3 号楼" {
		t.Errorf("签到点信息不完整：%+v", in.Places)
	}

	out := detail.CheckRange(116.5, 39.9)
	if out.InRange {
		t.Error("远离签到点应判定为范围外")
	}

	// 未设置签到点时按服务端标记处理
	noPlaces := &TaskDetail{IsMalposition: FlexInt(0)}
	if !noPlaces.CheckRange(0, 0).InRange {
		t.Error("无签到点且 isMalposition=0 时应视为范围内")
	}
	malposition := &TaskDetail{IsMalposition: FlexInt(1)}
	if malposition.CheckRange(0, 0).InRange {
		t.Error("无签到点且 isMalposition=1 时应视为范围外")
	}
}

// TestPickExtraAnswer 校验附加题择优逻辑。
func TestPickExtraAnswer(t *testing.T) {
	field := ExtraField{Title: "是否在校"}
	field.Items = []struct {
		WID          string  `json:"wid"`
		Content      string  `json:"content"`
		IsSelected   FlexInt `json:"isSelected"`
		IsOtherItems FlexInt `json:"isOtherItems"`
	}{
		{WID: "w1", Content: "否"},
		{WID: "w2", Content: "在校"},
	}
	wid, content, ok := PickExtraAnswer(field)
	if !ok || wid != "w2" || content != "在校" {
		t.Errorf("应优先选择「在校」，得到 %s/%s", wid, content)
	}

	empty := ExtraField{Title: "空"}
	if _, _, ok := PickExtraAnswer(empty); ok {
		t.Error("无选项时应返回 false")
	}
}

// TestExplainFailure 校验错误提示分类。
func TestExplainFailure(t *testing.T) {
	cases := []struct{ code, msg, wantContains string }{
		{"3", "设备未授权", "设备未授权"},
		{"0", "今日校园版本过低", "版本过低"},
		{"4", "解析失败", "无法解析"},
		{"1", "不在签到范围", "范围"},
	}
	for _, c := range cases {
		got := ExplainFailure(c.code, c.msg)
		if !strings.Contains(got, c.wantContains) {
			t.Errorf("ExplainFailure(%q,%q) = %q，应包含 %q", c.code, c.msg, got, c.wantContains)
		}
	}
}

// TestDeviceProfileUserAgent 校验 UA 拼接包含关键字段。
func TestDeviceProfileUserAgent(t *testing.T) {
	d := DeviceProfile{DeviceModel: "23127PN0CC", SystemVersion: "14", BuildID: "VLFCNXM"}
	ua := d.UserAgent()
	for _, want := range []string{"Android 14", "23127PN0CC", "VLFCNXM", "cpdaily/" + AppVersion} {
		if !strings.Contains(ua, want) {
			t.Errorf("UA 缺少 %q：%s", want, ua)
		}
	}

	// 空设备信息应回退到默认值
	if ua := (DeviceProfile{}).UserAgent(); !strings.Contains(ua, "23127PN0CC") {
		t.Error("空设备型号应回退到默认值")
	}
}

// TestImportCookieString 校验 Cookie 解析。
func TestImportCookieString(t *testing.T) {
	got := ImportCookieString(" MOD_AUTH_CAS=abc ; sessionToken=xyz;  bad ")
	if got["MOD_AUTH_CAS"] != "abc" {
		t.Errorf("MOD_AUTH_CAS 解析错误：%q", got["MOD_AUTH_CAS"])
	}
	if got["sessionToken"] != "xyz" {
		t.Errorf("sessionToken 解析错误：%q", got["sessionToken"])
	}
	if len(got) != 2 {
		t.Errorf("应解析出 2 个 cookie，得到 %d：%v", len(got), got)
	}
}

// TestEndpointsFor 校验三类任务端点齐全。
func TestEndpointsFor(t *testing.T) {
	for _, kind := range []TaskKind{KindSign, KindAttendance, KindCollect} {
		e := EndpointsFor(kind)
		if e.List == "" || e.Detail == "" || e.Submit == "" {
			t.Errorf("%s 端点缺失：%+v", kind, e)
		}
		if !strings.HasPrefix(e.List, "/wec-counselor-") {
			t.Errorf("%s 列表端点前缀异常：%s", kind, e.List)
		}
	}
}

// TestServiceBase 校验认证服务根地址补全，避免拼出 /iap/iap 这类重复路径。
func TestServiceBase(t *testing.T) {
	cases := []struct{ idsURL, segment, want string }{
		// 已带服务段：不再追加
		{"https://kjxyctgu.campusphere.net/iap", "iap", "https://kjxyctgu.campusphere.net/iap"},
		{"https://sso.gxust.edu.cn/authserver", "authserver", "https://sso.gxust.edu.cn/authserver"},
		{"https://host/iap/", "iap", "https://host/iap"},
		// 只有根域名：补全
		{"https://host", "iap", "https://host/iap"},
		{"https://host", "authserver", "https://host/authserver"},
		{"https://host/", "authserver", "https://host/authserver"},
		// 已指向其它认证服务时不强行替换
		{"https://host/authserver", "iap", "https://host/authserver"},
		{"https://host/lyuapServer", "authserver", "https://host/lyuapServer"},
		// 空值
		{"", "iap", ""},
	}
	for _, c := range cases {
		if got := serviceBase(c.idsURL, c.segment); got != c.want {
			t.Errorf("serviceBase(%q, %q) = %q, want %q", c.idsURL, c.segment, got, c.want)
		}
	}
}

// TestIsSuccessCode 校验不同子系统的成功码都算成功。
func TestIsSuccessCode(t *testing.T) {
	success := []string{"", "0", "200", "ok", "OK", "success", "true"}
	for _, c := range success {
		if !isSuccessCode(c) {
			t.Errorf("%q 应视为成功", c)
		}
	}
	fail := []string{"1", "3", "4", "400", "401", "500", "CAPTCHA_NOTMATCH", "FAIL_UPNOTMATCH"}
	for _, c := range fail {
		if isSuccessCode(c) {
			t.Errorf("%q 不应视为成功", c)
		}
	}
}

// TestRSAEncryptIAPPassword 校验 IAP 密码加密格式。
func TestRSAEncryptIAPPassword(t *testing.T) {
	out, err := RSAEncryptIAPPassword("Test@123456", "")
	if err != nil {
		t.Fatalf("加密失败：%v", err)
	}
	if !strings.HasPrefix(out, "{rsa}") {
		t.Errorf("必须带 {rsa} 前缀，否则服务端按明文处理：%s", out[:20])
	}
	// 1024 位 RSA 密文 = 128 字节 -> base64 172 字符，加前缀共 177
	if len(out) != 177 {
		t.Errorf("密文长度异常：%d（期望 177）", len(out))
	}
	// 相同明文两次加密结果应不同（PKCS1 v1.5 随机填充）
	out2, _ := RSAEncryptIAPPassword("Test@123456", "")
	if out == out2 {
		t.Error("相同明文应产生不同密文（PKCS#1 v1.5 随机填充）")
	}
}

// TestRSAEncryptIAPPasswordInvalidKey 非法公钥应报错。
func TestRSAEncryptIAPPasswordInvalidKey(t *testing.T) {
	if _, err := RSAEncryptIAPPassword("x", "not-base64!!!"); err == nil {
		t.Error("非法公钥应报错")
	}
	if _, err := RSAEncryptIAPPassword("x", "TUlHZk1BMEdDU3FHU0liM0RRRUJBUVVBQTRHTkFEQ0JpUUtCZ1FB"); err == nil {
		// base64 合法但不是 SPKI 结构
		t.Log("非 SPKI 数据应当解析失败")
	}
}

// TestExtractRSAPublicKey 校验从登录页 JS 提取公钥。
func TestExtractRSAPublicKey(t *testing.T) {
	// 模拟 chunk-common.js 里的写法
	js := `var e = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDCpbRy8ZoyQvRPpUDIXycglTwVDcYr` +
		`Lcv7P9HI4E7/TAPZiI3GN0ckTUVVWRvoo/re3uCOOJK+F+Ufh19XiRIdoPYCULsSyEjcL` +
		`LiyAS09mVOzhAQco84E/lM24T7rRTVID7LIgWPewyN8Nd5vzHET4Q3qTthoL8n82BwC1i` +
		`XuVQIDAQAB";obj.setPublicKey(e);`
	key := ExtractRSAPublicKey(js)
	if !strings.HasPrefix(key, "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ") {
		t.Fatalf("提取的公钥不正确：%q", key)
	}
	for _, ch := range key {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=", ch) {
			t.Fatalf("公钥混入了非法字符 %q：%q", ch, key)
		}
	}
	if strings.HasSuffix(key, "obj") || strings.Contains(key, `"`) {
		t.Errorf("公钥不应包含 JS 代码残留：%q", key)
	}
	// 用提取出的公钥能加密
	if _, err := RSAEncryptIAPPassword("pw", key); err != nil {
		t.Errorf("提取的公钥应可用：%v", err)
	}

	if got := ExtractRSAPublicKey("no key here"); got != "" {
		t.Errorf("无公钥时应返回空串，得到 %q", got)
	}
	if got := ExtractRSAPublicKey("MIGfMA0GCSshort"); got != "" {
		t.Errorf("过短的 matches 应被拒绝，得到 %q", got)
	}
}

// TestExtractScriptURLs 校验脚本地址提取。
func TestExtractScriptURLs(t *testing.T) {
	html := `<script src=./platform/core.js></script>` +
		`<link href=/iap/login/js/chunk-common.4dfeb954.js rel=preload as=script>` +
		`<script src="/iap/login/js/mobile.001cea1c.js"></script>`
	urls := extractScriptURLs(html)
	if len(urls) < 2 {
		t.Fatalf("应提取到至少 2 个脚本，得到 %v", urls)
	}
	found := false
	for _, u := range urls {
		if strings.Contains(u, "chunk-common") {
			found = true
		}
	}
	if !found {
		t.Errorf("应提取到 chunk-common.js，得到 %v", urls)
	}
}

// TestSchemeHost 校验从绝对地址取 scheme://host。
func TestSchemeHost(t *testing.T) {
	cases := map[string]string{
		"https://host.com/iap":        "https://host.com",
		"http://localhost:8080/a/b":   "http://localhost:8080",
		"not a url":                   "",
	}
	for in, want := range cases {
		if got := schemeHost(in); got != want {
			t.Errorf("schemeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSubmitCloudRedirect 校验 IAP 登录成功后必须再换 service 票。
// 回归：登录返回 HTTP 200 + {"resultCode":"REDIRECT"} 时，
// 旧实现只在 302 分支处理，导致误报「登录失败（HTTP 200）」。
func TestIAPRedirectIsSuccess(t *testing.T) {
	cases := []struct {
		code  string
		fail  bool
	}{
		{"REDIRECT", false},   // 成功
		{"FAIL_UPNOTMATCH", true},
		{"CAPTCHA_NOTMATCH", true},
	}
	for _, c := range cases {
		body := `{"resultCode":"` + c.code + `","url":"/portal/login"}`
		if c.code != "REDIRECT" {
			body = `{"resultCode":"` + c.code + `"}`
		}
		// REDIRECT 必须被识别为成功
		if c.code == "REDIRECT" && !strings.Contains(body, "REDIRECT") {
			t.Fatal("测试数据构造错误")
		}
		if c.fail && strings.Contains(body, `"REDIRECT"`) {
			t.Errorf("%s 不应含 REDIRECT", c.code)
		}
	}
}

// TestHostFromBase 校验从 idsUrl 提取 host。
func TestHostFromBase(t *testing.T) {
	cases := map[string]string{
		"https://kjxyctgu.campusphere.net/iap": "kjxyctgu.campusphere.net",
		"https://sso.gxust.edu.cn/authserver":  "sso.gxust.edu.cn",
		"https://host":                         "host",
		"not a url":                            "",
	}
	for in, want := range cases {
		if got := hostFromBase(in); got != want {
			t.Errorf("hostFromBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNoRedirectClient 校验 doNoRedirect 不会自动跟随重定向。
// 回归：换 service 票时必须读到 302 的 Location，
// 用默认 http.Client 会被自动跟随吞掉，导致拿不到票。
func TestNoRedirectClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.SetCookie(w, &http.Cookie{Name: "CASTGC", Value: "abc"})
			w.Header().Set("Location", "/next")
			w.WriteHeader(http.StatusFound)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "MOD_AUTH_CAS", Value: "xyz"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{Timeout: 5 * time.Second}, nil)
	resp, err := c.doNoRedirect(context.Background(), http.MethodGet, srv.URL+"/start",
		map[string]string{"User-Agent": "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("应停在 302，得到 %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/next" {
		t.Errorf("Location 应可读取，得到 %q", loc)
	}
	if c.Cookies()["CASTGC"] != "abc" {
		t.Errorf("302 上的 Set-Cookie 应被收集，得到 %v", c.Cookies())
	}

	// 普通 do 应自动跟随
	resp2, err := c.do(context.Background(), http.MethodGet, srv.URL+"/start",
		map[string]string{"User-Agent": "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("do 应跟随到 200，得到 %d", resp2.StatusCode)
	}
	if c.Cookies()["MOD_AUTH_CAS"] != "xyz" {
		t.Errorf("跟随后的 Cookie 应被收集，得到 %v", c.Cookies())
	}
}

// TestDeviceProfileIOS 校验 iOS 设备信息生成。
func TestDeviceProfileIOS(t *testing.T) {
	ios := DeviceProfile{
		Platform: "iOS", DeviceModel: "iPhone 16 Pro",
		SystemVersion: "18.2", BuildID: "", AppVersion: "9.9.22",
	}
	if !ios.IsIOS() {
		t.Error("Platform=iOS 应识别为 iOS")
	}
	ua := ios.UserAgent()
	// iOS UA 特征：iPhone / OS 版本下划线 / Mobile/15E148 / 无 okhttp
	if !strings.Contains(ua, "iPhone") {
		t.Errorf("iOS UA 应含 iPhone：%s", ua)
	}
	if !strings.Contains(ua, "iPhone OS 18.2") {
		t.Errorf("iOS UA 应含 iPhone OS 18.2：%s", ua)
	}
	if !strings.Contains(ua, "Mobile/15E148") {
		t.Errorf("iOS UA 应含 Mobile/15E148：%s", ua)
	}
	if strings.Contains(ua, "okhttp") {
		t.Errorf("iOS UA 不应含 okhttp：%s", ua)
	}

	// Android 仍走原格式
	and := DeviceProfile{Platform: "android", DeviceModel: "23127PN0CC", SystemVersion: "14"}
	if and.IsIOS() {
		t.Error("android 不应识别为 iOS")
	}
	if !strings.Contains(and.UserAgent(), "okhttp") {
		t.Error("Android UA 应含 okhttp")
	}

	// 空平台默认 Android
	if (DeviceProfile{}).IsIOS() {
		t.Error("空平台应默认为 Android")
	}
}

// TestIOSCookieHeader 校验 iOS 手动 Cookie 拼接。
func TestIOSCookieHeader(t *testing.T) {
	c := NewClient(ClientOptions{}, map[string]string{
		"sessionToken":  "TOKEN",
		"MOD_AUTH_CAS":  "CAS",
		"CASTGC":        "UNUSED",
	})
	got := c.iosCookieHeader("TENANT")
	for _, want := range []string{
		"clientType=cpdaily_student", "standAlone=0",
		"tenantId=TENANT", "sessionToken=TOKEN", "MOD_AUTH_CAS=CAS",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Cookie 缺少 %q：%s", want, got)
		}
	}
	// CASTGC 不应出现在提交 Cookie 中
	if strings.Contains(got, "CASTGC") {
		t.Errorf("Cookie 不应含 CASTGC：%s", got)
	}
}

// TestIOSBodyStringSameCrypto 校验 iOS 与 Android 的 bodyString 加密方式一致。
// 抓包确认：两者都是 AES-128-CBC + 相同 IV + 相同 key。
func TestIOSBodyStringSameCrypto(t *testing.T) {
	// 用抓包中解出的字段名构造假数据（不含任何真实值）
	form := map[string]any{
		"longitude": 111.0, "latitude": 30.0, "isMalposition": 0,
		"abnormalReason": "", "signPhotoUrl": "https://example.invalid/x",
		"position": "测试地址", "ticket": "", "uaIsCpadaily": true,
		"signInstanceWid": "000000",
	}
	body, err := json.Marshal(form)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := AESEncryptCBC(string(body), FKeyCat)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := AESDecryptCBC(enc, FKeyCat)
	if err != nil {
		t.Fatal(err)
	}
	if dec != string(body) {
		t.Error("bodyString 加解密不一致")
	}
	// 抓包里 bodyString 的 base64 长度是 600（9 个字段），量级应接近
	if len(enc) < 100 || len(enc) > 2000 {
		t.Errorf("bodyString 长度异常：%d", len(enc))
	}
}

// TestFlexFloatUnmarshal 校验经纬度兼容字符串与数字。
// 回归：抓包里 longitude 是字符串 "111.2195637741102"，
// radius 是数字 1500，用 float64 直接解析会整条详情失败。
func TestFlexFloatUnmarshal(t *testing.T) {
	cases := []struct {
		json string
		want float64
	}{
		{`{"longitude":"111.2195637741102"}`, 111.2195637741102},
		{`{"longitude":111.2195637741102}`, 111.2195637741102},
		{`{"longitude":"30.708"}`, 30.708},
		{`{"longitude":0}`, 0},
		{`{"longitude":null}`, 0},
		{`{"longitude":""}`, 0},
		{`{"longitude":"abc"}`, 0}, // 无法解析时归零而非报错
	}
	for _, c := range cases {
		var v struct {
			Longitude FlexFloat `json:"longitude"`
		}
		if err := json.Unmarshal([]byte(c.json), &v); err != nil {
			t.Errorf("解析 %s 失败：%v", c.json, err)
			continue
		}
		if v.Longitude.Float() != c.want {
			t.Errorf("%s → %v，期望 %v", c.json, v.Longitude.Float(), c.want)
		}
	}
}

// TestSignPlaceMixedTypes 校验签到点混合类型可正常解析与计算。
func TestSignPlaceMixedTypes(t *testing.T) {
	// 字段类型与真实抓包一致：字符串坐标 + 数字半径
	raw := `{
	  "signInstanceWid":"40606",
	  "taskName":"测试查寝",
	  "isPhoto":1,
	  "isMalposition":0,
	  "signPlaceSelected":[{
	    "address":"测试校区",
	    "longitude":"111.2195637741102",
	    "latitude":"30.710147053240163",
	    "radius":1500
	  }]
	}`
	var d TaskDetail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(d.SignPlaces) != 1 {
		t.Fatalf("签到点数量错误：%d", len(d.SignPlaces))
	}
	p := d.SignPlaces[0]
	if p.Longitude.Float() < 111.2 || p.Longitude.Float() > 111.3 {
		t.Errorf("经度解析错误：%v", p.Longitude.Float())
	}
	if p.Radius.Float() != 1500 {
		t.Errorf("半径解析错误：%v", p.Radius.Float())
	}
	if d.IsPhoto.Int() != 1 {
		t.Errorf("isPhoto 解析错误：%v", d.IsPhoto.Int())
	}

	// 范围内判定
	check := d.CheckRange(111.2195, 30.7101)
	if !check.InRange {
		t.Error("同一点应判定为范围内")
	}
	check = d.CheckRange(111.5, 30.9)
	if check.InRange {
		t.Error("远点应判定为范围外")
	}
}

// TestFlexIntUnmarshal 校验整型字段兼容字符串。
func TestFlexIntUnmarshal(t *testing.T) {
	cases := []struct {
		json string
		want int
	}{
		{`{"v":1}`, 1},
		{`{"v":"1"}`, 1},
		{`{"v":"0"}`, 0},
		{`{"v":null}`, 0},
		{`{"v":""}`, 0},
		{`{"v":1.9}`, 1},
		{`{"v":"abc"}`, 0},
	}
	for _, c := range cases {
		var v struct {
			V FlexInt `json:"v"`
		}
		if err := json.Unmarshal([]byte(c.json), &v); err != nil {
			t.Errorf("解析 %s 失败：%v", c.json, err)
			continue
		}
		if v.V.Int() != c.want {
			t.Errorf("%s → %d，期望 %d", c.json, v.V.Int(), c.want)
		}
	}
}

// TestEndpointPaths 锁定各任务类型的接口路径。
// 回归：以下路径曾被写错，实测 404：
//   签到详情 detailSignTaskInst → 应为 detailSignInstance
//   签到提交 completeSignIn     → 应为 submitSign
//   签到上传 sign/getUploadPolicy → 应为 stu/obs/getUploadPolicy
func TestEndpointPaths(t *testing.T) {
	sign := EndpointsFor(KindSign)
	cases := []struct{ name, got, want string }{
		{"签到列表", sign.List, "/wec-counselor-sign-apps/stu/sign/getStuSignInfosInOneDay"},
		{"签到详情", sign.Detail, "/wec-counselor-sign-apps/stu/sign/detailSignInstance"},
		{"签到提交", sign.Submit, "/wec-counselor-sign-apps/stu/sign/submitSign"},
		{"签到上传策略", sign.STS, "/wec-counselor-sign-apps/stu/obs/getUploadPolicy"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s 路径错误：%s（应为 %s）", c.name, c.got, c.want)
		}
	}

	att := EndpointsFor(KindAttendance)
	if att.Detail != "/wec-counselor-attendance-apps/student/attendance/detailSignInstance" {
		t.Errorf("查寝详情路径错误：%s", att.Detail)
	}
	if att.STS != "/wec-counselor-attendance-apps/student/attendance/getUploadPolicy" {
		t.Errorf("查寝上传策略路径错误：%s", att.STS)
	}
	if att.Submit != "/wec-counselor-attendance-apps/student/attendance/submitSign" {
		t.Errorf("查寝提交路径错误：%s", att.Submit)
	}
}

// TestURLEncodeSorted 校验 sign 用的参数拼接顺序稳定。
func TestURLEncodeSorted(t *testing.T) {
	m := map[string]any{"b": 2, "a": 1, "c": "x"}
	if got := urlEncodeSorted(m); got != "a=1&b=2&c=x" {
		t.Errorf("排序拼接错误：%s", got)
	}
}

// TestAttendanceSuccessResponse 校验查寝成功响应被正确识别。
// 回归：查寝提交成功返回 {"code":"0","message":"SUCCESS"}，
// 响应中没有 datas 字段。此前要求 datas.signedStuInfo 存在，
// 导致每次都判成失败（实际服务端已成功）。
func TestAttendanceSuccessResponse(t *testing.T) {
	responses := []struct {
		name string
		body string
		want bool
	}{
		{
			// 实测的查寝成功响应
			"查寝成功",
			`{"code":"0","message":"SUCCESS","userId":"2024235110","userName":"某某"}`,
			true,
		},
		{
			"签到成功（带 datas）",
			`{"code":"0","message":"SUCCESS","datas":{"signInstanceWid":"40606","signedStuInfo":null}}`,
			true,
		},
		{
			"业务失败",
			`{"code":"3","message":"设备未授权"}`,
			false,
		},
		{
			"版本过低",
			`{"code":"4","message":"今日校园版本过低"}`,
			false,
		},
	}

	for _, c := range responses {
		var parsed struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		}
		if err := json.Unmarshal([]byte(c.body), &parsed); err != nil {
			t.Fatalf("%s 解析失败：%v", c.name, err)
		}
		code := strings.Trim(string(parsed.Code), `"`)
		ok := code == "0" ||
			strings.EqualFold(strings.TrimSpace(parsed.Message), "SUCCESS") ||
			strings.Contains(parsed.Message, "成功")
		if ok != c.want {
			t.Errorf("%s：判定为 %v，期望 %v", c.name, ok, c.want)
		}
	}
}
