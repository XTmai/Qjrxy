package cpdaily

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// 登录类型。
const (
	LoginTypeCAS   = "CAS"
	LoginTypeCloud = "CLOUD"
)

// LoginState 是登录过程中的中间状态（需要验证码时由服务端保存）。
type LoginState struct {
	Tenant      *Tenant
	Username    string
	Password    string
	LoginURL    string
	FormType    int // CAS 表单类型 0/1/2
	Params      map[string]string
	LT          string
	NeedCaptcha bool
	CaptchaURL  string
	Cookies     map[string]string
	CreatedAt   time.Time
}

// LoginChallenge 返回给前端：或者已经成功，或者需要验证码。
type LoginChallenge struct {
	Done        bool
	NeedCaptcha bool
	CaptchaKind string // "image"
	Captcha     []byte // PNG/JPEG 原始字节
	Message     string
	State       *LoginState
}

var errLoginFailed = errors.New("登录失败")

// StartLogin 执行到"需要验证码"或"登录完成"为止。
//
// 返回的 challenge.Done == true 表示已拿到会话 cookie（在 client 内）。
func StartLogin(ctx context.Context, opts ClientOptions, tenant *Tenant, username, password string) (*LoginChallenge, *Client, error) {
	client := NewClient(opts, nil)

	if strings.EqualFold(tenant.JoinType, LoginTypeCloud) {
		ch, err := startCloudLogin(ctx, client, tenant, username, password)
		return ch, client, err
	}
	ch, err := startCASLogin(ctx, client, tenant, username, password)
	return ch, client, err
}

// FinishLogin 用验证码完成登录，返回携带会话的新客户端。
func FinishLogin(ctx context.Context, opts ClientOptions, state *LoginState, code string) (*Client, error) {
	return FinishLoginWith(ctx, opts, state, code, nil)
}

// FinishLoginWith 在给定客户端上完成验证码登录（客户端需带上登录页 cookie）。
func FinishLoginWith(ctx context.Context, opts ClientOptions, state *LoginState, code string, client *Client) (*Client, error) {
	if client == nil {
		client = NewClient(opts, state.Cookies)
	}
	if strings.EqualFold(state.Tenant.JoinType, LoginTypeCloud) {
		return finishCloudLogin(ctx, client, state, code)
	}
	return finishCASLogin(ctx, client, state, code)
}

// ---------- CAS ----------

func randString(n int) string {
	const charset = "ABCDEFGHJKMNPQRSTWXYZabcdefhijkmnprstwxyz2345678"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = charset[i%len(charset)]
		}
		return string(buf)
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = charset[int(b)%len(charset)]
	}
	return string(out)
}

func startCASLogin(ctx context.Context, client *Client, tenant *Tenant, username, password string) (*LoginChallenge, error) {
	loginURL := tenant.LoginURL()
	resp, err := client.do(ctx, http.MethodGet, loginURL, map[string]string{
		"User-Agent": casUserAgent,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("打开登录页失败：%w", err)
	}
	defer resp.Body.Close()
	page, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	html := string(page)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("解析登录页失败：%w", err)
	}

	state := &LoginState{
		Tenant:    tenant,
		Username:  username,
		Password:  password,
		LoginURL:  loginURL,
		FormType:  0,
		Params:    map[string]string{},
		CreatedAt: time.Now(),
	}

	var formSel string
	switch {
	case doc.Find("#casLoginForm").Length() > 0:
		state.FormType = 0
		formSel = "#casLoginForm"
	case doc.Find("#pwdFromId").Length() > 0:
		state.FormType = 1
		formSel = "#pwdFromId"
	case doc.Find("#loginFromId").Length() > 0:
		state.FormType = 1
		sel := doc.Find("#loginFromId")
		if sel.Length() > 1 {
			formSel = "#loginFromId"
		} else {
			formSel = "#loginFromId"
		}
	case doc.Find("#fm1").Length() > 0:
		state.FormType = 2
		formSel = "#fm1"
	default:
		// 可能已经直接进入门户（cookie 已存在）或页面结构变化
		if strings.Contains(html, "欢迎") && client.hasSession() {
			return &LoginChallenge{Done: true, State: state}, nil
		}
		return nil, errors.New("登录页结构无法识别，可能是学校使用了新的认证方式")
	}

	form := doc.Find(formSel).First()
	form.Find("input").Each(func(_ int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok || name == "" || name == "rememberMe" {
			return
		}
		value, _ := s.Attr("value")
		state.Params[name] = value
	})
	state.Params["username"] = username

	if state.FormType == 2 {
		re := regexp.MustCompile(`RSAKeyPair\((.*?)\);`)
		m := re.FindStringSubmatch(html)
		if len(m) < 2 {
			return nil, errors.New("未找到 RSA 公钥配置")
		}
		parts := strings.Split(strings.ReplaceAll(m[1], `"`, ""), ",")
		if len(parts) < 3 {
			return nil, errors.New("RSA 公钥格式异常")
		}
		enc, err := rsaEncryptHex(password, parts[2], parts[0])
		if err != nil {
			return nil, err
		}
		state.Params["password"] = enc
		state.CaptchaURL = serviceBase(tenant.IDSURL, "lyuapServer") + "/captcha.jsp"
		state.NeedCaptcha = true
		state.Params["captcha"] = ""
		return &LoginChallenge{NeedCaptcha: true, CaptchaKind: "image", State: state}, nil
	}

	// 密码用页面上的 salt 做 AES 加密
	salt := ""
	if state.FormType == 0 {
		salt = strings.TrimSpace(doc.Find("#pwdDefaultEncryptSalt").AttrOr("value", ""))
	} else {
		salt = strings.TrimSpace(doc.Find("#pwdEncryptSalt").AttrOr("value", ""))
	}
	if salt == "" {
		if m := regexp.MustCompile(`"(\w{16})"`).FindStringSubmatch(html); len(m) == 2 {
			salt = m[1]
		}
	}
	if salt != "" {
		enc, err := AESEncryptCBC(randString(64)+password, salt)
		if err != nil {
			return nil, err
		}
		state.Params["password"] = enc
	} else {
		state.Params["password"] = password
	}

	state.NeedCaptcha = casNeedCaptcha(ctx, client, tenant, state.FormType, username)
	if state.NeedCaptcha {
		if state.FormType == 0 {
			state.CaptchaURL = serviceBase(tenant.IDSURL, "authserver") + "/captcha.html"
			state.Params["captchaResponse"] = ""
		} else {
			state.CaptchaURL = serviceBase(tenant.IDSURL, "authserver") + "/getCaptcha.htl"
			state.Params["captcha"] = ""
		}
		img, err := fetchCaptcha(ctx, client, state.CaptchaURL)
		if err != nil {
			return nil, err
		}
		return &LoginChallenge{NeedCaptcha: true, CaptchaKind: "image", Captcha: img, State: state}, nil
	}

	if err := submitCAS(ctx, client, tenant, state); err != nil {
		return nil, err
	}
	return &LoginChallenge{Done: true, State: state}, nil
}

// serviceBase 返回认证服务的根地址。
//
// 各校 idsUrl 的形态并不统一：有的直接给到服务根（https://host），
// 有的已经带上了服务段（https://host/iap、https://host/authserver）。
// 这里按服务名补全，避免拼出 /iap/iap/security/lt 这类重复路径。
func serviceBase(idsURL, segment string) string {
	base := strings.TrimRight(strings.TrimSpace(idsURL), "/")
	if base == "" {
		return base
	}
	if strings.HasSuffix(strings.ToLower(base), "/"+segment) {
		return base
	}
	// 已经指向其他认证服务时不要强行追加
	lower := strings.ToLower(base)
	if i := strings.LastIndex(lower, "/"); i > 0 {
		last := lower[i+1:]
		if last == "iap" || last == "authserver" || last == "lyuapserver" {
			return base
		}
	}
	return base + "/" + segment
}

const casUserAgent = "Mozilla/5.0 (Linux; Android 8.0.0; MI 6 Build/OPR1.170623.027; wv) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/92.0.4515.131 Mobile Safari/537.36 okhttp/3.12.4"

func casNeedCaptcha(ctx context.Context, client *Client, tenant *Tenant, formType int, username string) bool {
	base := serviceBase(tenant.IDSURL, "authserver")
	var target string
	if formType == 0 {
		target = base + "/needCaptcha.html?username=" + url.QueryEscape(username)
	} else {
		target = base + "/checkNeedCaptcha.htl?username=" + url.QueryEscape(username)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", casUserAgent)
	req.Header.Set("Cookie", client.CookieString())
	resp, err := client.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	client.absorb(resp)
	text := strings.TrimSpace(string(body))
	if formType == 0 {
		lower := strings.ToLower(text)
		return !strings.HasPrefix(lower, "false")
	}
	return strings.Contains(text, "true")
}

func fetchCaptcha(ctx context.Context, client *Client, imgURL string) ([]byte, error) {
	resp, err := client.do(ctx, http.MethodGet, imgURL, map[string]string{"User-Agent": casUserAgent}, nil)
	if err != nil {
		return nil, fmt.Errorf("获取验证码失败：%w", err)
	}
	defer resp.Body.Close()
	img, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if len(img) == 0 {
		return nil, errors.New("验证码图片为空")
	}
	return img, nil
}

func submitCAS(ctx context.Context, client *Client, tenant *Tenant, state *LoginState) error {
	form := url.Values{}
	for k, v := range state.Params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, state.LoginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", casUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", client.CookieString())
	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("提交登录请求失败：%w", err)
	}
	client.absorb(resp)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if location != "" {
			follow, err := client.do(ctx, http.MethodGet, location, map[string]string{"User-Agent": casUserAgent}, nil)
			if err != nil {
				return fmt.Errorf("跟随登录跳转失败：%w", err)
			}
			follow.Body.Close()
			if strings.Contains(follow.Request.URL.Host, "campusphere.net") ||
				strings.Contains(follow.Request.URL.Host, tenant.Host) || client.hasSession() {
				return nil
			}
		}
		if client.hasSession() {
			return nil
		}
		return errors.New("CAS 登录未完成跳转，请稍后重试")
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if msg := extractCASError(string(body)); msg != "" {
		return fmt.Errorf("登录失败：%s", msg)
	}
	if client.hasSession() {
		return nil
	}
	return fmt.Errorf("登录失败（HTTP %d），请检查账号密码", resp.StatusCode)
}

func extractCASError(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	for _, sel := range []string{"#errorMsg", "#formErrorTip2", "#msg", ".auth_error", "#msgDiv"} {
		if s := doc.Find(sel).First(); s.Length() > 0 {
			if text := strings.TrimSpace(s.Text()); text != "" {
				return text
			}
		}
	}
	return ""
}

func finishCASLogin(ctx context.Context, client *Client, state *LoginState, code string) (*Client, error) {
	// 验证码会话 cookie 由上层在重建 Client 时注入
	if code != "" {
		if state.FormType == 0 {
			state.Params["captchaResponse"] = code
		} else {
			state.Params["captcha"] = code
		}
	}
	if err := submitCAS(ctx, client, state.Tenant, state); err != nil {
		return nil, err
	}
	return client, nil
}

// rsaEncryptHex 按页面给出的模数/指数（十六进制）做 PKCS#1 v1.5 加密，输出 hex。
func rsaEncryptHex(plaintext, modulusHex, exponentHex string) (string, error) {
	modulus := new(big.Int)
	if _, ok := modulus.SetString(modulusHex, 16); !ok {
		return "", errors.New("RSA 模数解析失败")
	}
	exp := new(big.Int)
	if _, ok := exp.SetString(exponentHex, 16); !ok {
		return "", errors.New("RSA 指数解析失败")
	}
	pub := &rsa.PublicKey{N: modulus, E: int(exp.Int64())}
	out, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(out), nil
}

// ---------- IAP（login_type = CLOUD） ----------

func startCloudLogin(ctx context.Context, client *Client, tenant *Tenant, username, password string) (*LoginChallenge, error) {
	base := serviceBase(tenant.IDSURL, "iap")
	state := &LoginState{
		Tenant:    tenant,
		Username:  username,
		Password:  password,
		LoginURL:  base + "/doLogin",
		Params:    map[string]string{},
		CreatedAt: time.Now(),
	}

	// 1. 取 _lt
	ltURL := base + "/security/lt"
	ltResp, err := client.postJSON(ctx, ltURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("获取 lt 失败：%w", err)
	}
	var ltPayload struct {
		Result struct {
			LT          string `json:"_lt"`
			EncryptSalt string `json:"_encryptSalt"`
			NeedCaptcha bool   `json:"needCaptcha"`
		} `json:"result"`
	}
	if err := jsonUnmarshal(ltResp.Raw, &ltPayload); err != nil {
		return nil, err
	}
	state.LT = ltPayload.Result.LT
	if state.LT == "" {
		return nil, fmt.Errorf("登录服务未返回 lt，请稍后重试")
	}
	state.Params["lt"] = state.LT
	state.Params["rememberMe"] = "false"
	state.Params["dllt"] = ""
	state.Params["mobile"] = ""
	state.Params["username"] = username
	state.Params["password"] = password

	// 网页版 IAP 的密码格式是 "{rsa}" + base64(RSA_PKCS1v15(password))。
	// 公钥写在登录页 chunk-common.js 里，各校可能不同，优先动态抓取；
	// 抓不到时回退内置公钥。缺少 {rsa} 前缀会被当成明文，服务端返回 FAIL_UPNOTMATCH。
	pubKey := fetchIAPPublicKey(ctx, client, base)
	encPwd, err := RSAEncryptIAPPassword(password, pubKey)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败：%w", err)
	}
	state.Params["password"] = encPwd
	_ = ltPayload.Result.EncryptSalt

	// 2. 是否需要验证码：lt 响应里已经带了一次，再单独查一次兜底
	needCaptcha := ltPayload.Result.NeedCaptcha
	if !needCaptcha {
		needURL := base + "/checkNeedCaptcha?username=" + url.QueryEscape(username)
		if needResp, err := client.postJSON(ctx, needURL, nil, nil); err == nil {
			var probe struct {
				NeedCaptcha bool `json:"needCaptcha"`
			}
			if jsonUnmarshal(needResp.Raw, &probe) == nil {
				needCaptcha = probe.NeedCaptcha
			}
		}
	}
	state.NeedCaptcha = needCaptcha
	if needCaptcha {
		state.CaptchaURL = base + "/generateCaptcha?ltId=" + url.QueryEscape(state.LT)
		state.Params["captcha"] = ""
		img, err := fetchCaptcha(ctx, client, state.CaptchaURL)
		if err != nil {
			return nil, err
		}
		return &LoginChallenge{NeedCaptcha: true, CaptchaKind: "image", Captcha: img, State: state}, nil
	}

	state.Params["captcha"] = ""
	if err := submitCloud(ctx, client, tenant, state); err != nil {
		return nil, err
	}
	return &LoginChallenge{Done: true, State: state}, nil
}

func finishCloudLogin(ctx context.Context, client *Client, state *LoginState, code string) (*Client, error) {
	state.Params["captcha"] = code
	if err := submitCloud(ctx, client, state.Tenant, state); err != nil {
		return nil, err
	}
	return client, nil
}

// fetchIAPPublicKey 从登录页 JS 里抓取 RSA 公钥，失败时返回空串（走内置兜底）。
func fetchIAPPublicKey(ctx context.Context, client *Client, base string) string {
	// 登录页 HTML 里列出了 chunk-common.js 的地址
	pageURL := base + "/login/mobile.html"
	resp, err := client.do(ctx, http.MethodGet, pageURL, map[string]string{"User-Agent": casUserAgent}, nil)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	html, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))

	// 路径是相对当前目录的，如 /iap/login/js/chunk-common.xxx.js
	candidates := extractScriptURLs(string(html))
	var lastJS string
	for _, src := range candidates {
		full := src
		if strings.HasPrefix(src, "./") {
			full = base + "/login/" + src[2:]
		} else if strings.HasPrefix(src, "/") {
			full = schemeHost(base) + src
		} else if !strings.HasPrefix(src, "http") {
			continue
		}
		jsResp, err := client.do(ctx, http.MethodGet, full, map[string]string{"User-Agent": casUserAgent}, nil)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(jsResp.Body, 4<<20))
		jsResp.Body.Close()
		lastJS = string(body)
		if strings.Contains(lastJS, "{rsa}") || strings.Contains(lastJS, "setPublicKey") {
			if key := ExtractRSAPublicKey(lastJS); key != "" {
				return key
			}
		}
	}
	if lastJS != "" {
		return ExtractRSAPublicKey(lastJS)
	}
	return ""
}

// extractScriptURLs 提取 HTML 中的脚本地址。
//
// 除了 <script src>，还要处理 <link href=... as=script> 的预加载声明——
// 今日校园的登录页把包含 RSA 公钥的 chunk-common.js 放在 preload 里，
// 只匹配 src 会漏掉它。
func extractScriptURLs(html string) []string {
	re := regexp.MustCompile(`(?:src|href)=["']?([^"'\s>]+\.js[^"'\s>]*)["']?`)
	matches := re.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

// schemeHost 从绝对 URL 中取出 scheme://host。
func schemeHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func submitCloud(ctx context.Context, client *Client, tenant *Tenant, state *LoginState) error {
	base := serviceBase(tenant.IDSURL, "iap")
	form := url.Values{}
	for k, v := range state.Params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/doLogin?r="+fmt.Sprintf("%d", time.Now().UnixMilli()),
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", casUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", client.CookieString())
	// 关键：没有这个头，IAP 会返回 302 + HTML 登录页而不是 JSON，
	// 导致无法解析 resultCode，登录结果被误判为失败。
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("提交登录失败：%w", err)
	}
	client.absorb(resp)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		loc := resp.Header.Get("Location")
		if loc != "" {
			follow, err := client.do(ctx, http.MethodGet, loc, map[string]string{"User-Agent": casUserAgent}, nil)
			if err != nil {
				return err
			}
			follow.Body.Close()
			return nil
		}
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if len(body) > 0 && strings.Contains(string(body), "resultCode") {
		var payload struct {
			ResultCode string `json:"resultCode"`
			Message    string `json:"message"`
			URL        string `json:"url"`
		}
		if jsonUnmarshal(body, &payload) == nil {
			switch payload.ResultCode {
			case "REDIRECT":
				// 登录成功：IAP 返回 HTTP 200 + {"resultCode":"REDIRECT","url":"/portal/login"}，
				// 此时 CASTGC 已种下，但还没有业务域的 MOD_AUTH_CAS，
				// 必须再换一次 service 票才能访问今日校园接口。
				if err := finishIAPServiceTicket(ctx, client, base); err != nil {
					return err
				}
				return nil
			case "CAPTCHA_NOTMATCH":
				return errors.New("验证码错误或已过期，请重新登录获取新验证码")
			case "FAIL_UPNOTMATCH":
				return errors.New("用户名或密码错误（账号不存在，或密码不正确）")
			case "CAPTCHA_ISNULL":
				return errors.New("学校要求输入验证码，请重新登录后输入")
			default:
				if payload.Message != "" {
					return fmt.Errorf("登录失败：%s", payload.Message)
				}
				return fmt.Errorf("登录失败：%s", payload.ResultCode)
			}
		}
	}
	if client.hasSession() {
		return nil
	}
	return fmt.Errorf("登录失败（HTTP %d）：%s", resp.StatusCode, truncate(string(body), 120))
}

// finishIAPServiceTicket 用已种下的 CASTGC 换业务域会话票。
//
// IAP 登录成功后只拿到 CAS 域的 CASTGC，访问 campusphere 业务接口还需要
// 用 service 参数再跳一次，让业务域种下 MOD_AUTH_CAS。
func finishIAPServiceTicket(ctx context.Context, client *Client, idpBase string) error {
	host := hostFromBase(idpBase)
	if host == "" {
		return nil
	}
	service := "https://" + host + "/wec-counselor-attendance-apps/student/mobileStudent/index.html"
	loginURL := idpBase + "/login?service=" + url.QueryEscape(service)

	// 第一次：拿 302 与业务域 service 票。
	// 必须用不自动跟随的客户端，否则 Location 会被 http.Client 消费掉。
	resp, err := client.doNoRedirect(ctx, http.MethodGet, loginURL,
		map[string]string{"User-Agent": casUserAgent}, nil)
	if err != nil {
		return fmt.Errorf("换取业务会话失败：%w", err)
	}
	location := resp.Header.Get("Location")
	resp.Body.Close()

	if location == "" {
		// 没有跳转说明 CASTGC 没生效，直接报错而不是静默通过
		return errors.New("换取业务会话失败：登录凭证未生效，请重新登录")
	}

	// 第二次：带着 ticket 访问业务域，让服务端种下 MOD_AUTH_CAS。
	// 同样不能用自动跟随的客户端：业务域返回 302 时就带着 Set-Cookie，
	// 若自动跟随到后续页面，Cookie 可能被后半段的响应覆盖。
	follow, err := client.doNoRedirect(ctx, http.MethodGet, location,
		map[string]string{"User-Agent": casUserAgent}, nil)
	if err != nil {
		return fmt.Errorf("访问业务域失败：%w", err)
	}
	follow.Body.Close()

	if _, ok := client.Cookies()["MOD_AUTH_CAS"]; !ok {
		return errors.New("登录已通过，但未拿到业务会话（MOD_AUTH_CAS），请重试")
	}
	return nil
}

// hostFromBase 从 https://host/path 中取出 host。
func hostFromBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
