package cpdaily

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// uaApp 是今日校园 App 的 User-Agent。
const uaApp = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 cpdaily/9.9.26 wisedu/9.9.26"

// ---------- QR 扫码登录 ----------

// QRLoginParams 是学校入口 302 后拿到的 OAuth 参数。
type QRLoginParams struct {
	ClientID    string `json:"clientId"`
	RedirectURI string `json:"redirectUri"`
	Scope       string `json:"scope"`
	State       string `json:"state"`
}

// jsLoginBody 是 connect/qrcode/jsLogin 的请求体。
type jsLoginBody struct {
	ClientID         string `json:"clientId"`
	RedirectURI      string `json:"redirectUri"`
	Scope            string `json:"scope"`
	ResponseResponse string `json:"responseType"`
	State            string `json:"state"`
	SupportFreeLogin bool   `json:"supportFreeLogin"`
}

// jsLoginResp 是 jsLogin 的响应。
type jsLoginResp struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    struct {
		NoLogin int    `json:"noLogin"`
		QRId    string `json:"qrId"`
	} `json:"data"`
}

// validationResp 是 validation/{qrId} 的响应。
type validationResp struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    struct {
		Status      int    `json:"status"` // 1=waiting 2/3=scanned 4=confirmed 5=expired
		RedirectURL string `json:"redirectUrl"`
	} `json:"data"`
}

// QRCodeAPIError 表示 campushoy 接口明确返回了错误（errCode != 0）。
// 与网络层错误区分：前者应当终止轮询并提示用户，后者可以继续等。
type QRCodeAPIError struct {
	Code int
	Msg  string
}

func (e *QRCodeAPIError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Msg)
}

// QRLoginPollResult 是一次轮询的结果。
type QRLoginPollResult struct {
	Status      int    `json:"status"` // 同 validation.status
	RedirectURL string `json:"redirectUrl,omitempty"`
}

// QRLoginCookies 是扫码完成后拿到的登录态。
type QRLoginCookies struct {
	// Cookies 是按域名去重的业务 Cookie（MOD_AUTH_CAS / CASTGC / cpdailySession 等）。
	// key 为 "domain|name"，value 为 Cookie 值。
	Cookies map[string]string `json:"cookies"`
	// TenantHost 是从学校域名的 Cookie 推断出的学校 host（如 kjxyctgu.campusphere.net）。
	TenantHost string `json:"tenantHost"`
	// TGC 是 CASTGC 的原始值（用于续期），可能为空。
	TGC string `json:"tgc,omitempty"`
}

const qrBaseURL = "https://api.campushoy.com"

// 学生端 H5 入口，uniauth 用它生成扫码登录的 OAuth 参数。
const (
	uniauthBaseURL = "https://uniauth.campusphere.net"
	qrStuService   = "/wec-counselor-stu-apps/stu/mobile/index.html"
	// defaultQRAppID 是实测可用的 uniauth 应用 ID。
	// 扫码后的学校由 App 当前登录的账号决定，不依赖这个 appId。
	defaultQRAppID = "d7de360368c94a808225"
)

// GetQROAuthParamsFromUniauth 通过 uniauth 的 getQrcodePageUrl 生成 OAuth 参数。
//
// 这是实测最稳的方式：不依赖学校入口 302 的形态，CLOUD / NOTCLOUD 都适用。
func GetQROAuthParamsFromUniauth(ctx context.Context, appID string) (*QRLoginParams, error) {
	if strings.TrimSpace(appID) == "" {
		appID = defaultQRAppID
	}
	body, _ := json.Marshal(map[string]string{
		"appId":   appID,
		"service": qrStuService,
	})
	req, err := http.NewRequestWithContext(ctx, "POST",
		uniauthBaseURL+"/wec-devops-school-auth/login/getQrcodePageUrl",
		strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", uaApp)

	resp, err := httpDo(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("获取扫码授权地址失败: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		Code   string `json:"code"`
		ErrMsg string `json:"message"`
		Datas  struct {
			QrcodePageURL string `json:"qrcodePageUrl"`
		} `json:"datas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("解析扫码授权地址失败: %w", err)
	}
	if out.Datas.QrcodePageURL == "" {
		return nil, fmt.Errorf("扫码授权地址为空（%s）", out.ErrMsg)
	}

	u, err := url.Parse(out.Datas.QrcodePageURL)
	if err != nil {
		return nil, fmt.Errorf("解析扫码授权地址失败: %w", err)
	}
	q := u.Query()
	if q.Get("client_id") == "" || q.Get("redirect_uri") == "" {
		return nil, fmt.Errorf("扫码授权地址缺少 OAuth 参数")
	}
	return &QRLoginParams{
		ClientID:    q.Get("client_id"),
		RedirectURI: q.Get("redirect_uri"),
		Scope:       q.Get("scope"),
		State:       q.Get("state"),
	}, nil
}

// GetQROAuthParams 访问学校入口页，跟随 302 提取 OAuth 参数。
//
// 入口 URL 使用辅导猫学生端 H5（所有学校类型统一）。
// CLOUD 学校会 302 到 IAP → OAuth authorize；
// NOTCLOUD 学校会 302 到学校 authserver → OAuth authorize。
// 最终都会落到 api.campushoy.com/connect/oauth2/authorize?...，从中提取参数。
func GetQROAuthParams(ctx context.Context, host string) (*QRLoginParams, error) {
	if strings.TrimSpace(host) == "" {
		return nil, fmt.Errorf("学校 host 不能为空")
	}
	entryURL := fmt.Sprintf("https://%s/wec-counselor-stu-apps/stu/mobile/index.html", host)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", entryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaApp)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("访问学校入口失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// 从最终 URL 的查询参数里提取 OAuth 信息
	finalURL := resp.Request.URL
	q := finalURL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	scope := q.Get("scope")
	state := q.Get("state")

	if clientID == "" || redirectURI == "" {
		// 某些学校可能直接落在登录页而非 OAuth authorize，
		// 此时从 Cookie 或 HTML 里找。目前先返回错误让上层处理。
		return nil, fmt.Errorf("未能从学校入口提取 OAuth 参数（client_id 或 redirect_uri 为空）")
	}

	return &QRLoginParams{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scope:       scope,
		State:       state,
	}, nil
}

// QRLoginStart 调用 connect/qrcode/jsLogin 获取 qrId。
func QRLoginStart(ctx context.Context, params *QRLoginParams) (qrId string, err error) {
	body := jsLoginBody{
		ClientID:         params.ClientID,
		RedirectURI:      params.RedirectURI,
		Scope:            params.Scope,
		ResponseResponse: "code",
		State:            params.State,
		SupportFreeLogin: true,
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", qrBaseURL+"/connect/qrcode/jsLogin", strings.NewReader(string(buf)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", uaApp)

	resp, err := httpDo(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res jsLoginResp
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("解析 jsLogin 响应失败: %w", err)
	}
	if res.ErrCode != 0 {
		return "", fmt.Errorf("jsLogin 失败: [%d] %s", res.ErrCode, res.ErrMsg)
	}
	if res.Data.QRId == "" {
		return "", fmt.Errorf("jsLogin 返回 qrId 为空")
	}
	return res.Data.QRId, nil
}

// QRLoginPoll 轮询 validation/{qrId}。
func QRLoginPoll(ctx context.Context, qrId string) (*QRLoginPollResult, error) {
	pollURL := fmt.Sprintf(qrBaseURL+"/connect/qrcode/validation/%s", qrId)
	req, err := http.NewRequestWithContext(ctx, "POST", pollURL, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", uaApp)

	resp, err := httpDo(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res validationResp
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("解析 validation 响应失败: %w", err)
	}
	// errCode 非 0 说明服务端拒绝了这次轮询（票据失效、参数错误等）。
	// 不读它的话 status 落到零值 0，上层会把错误当成「还在等待」无限轮询下去。
	if res.ErrCode != 0 {
		return nil, &QRCodeAPIError{Code: res.ErrCode, Msg: res.ErrMsg}
	}
	return &QRLoginPollResult{
		Status:      res.Data.Status,
		RedirectURL: res.Data.RedirectURL,
	}, nil
}

// QRLoginFollow 跟随 redirectUrl 跳转链，收集所有 Cookie。
//
// 使用 http.Client + CookieJar 自动跟随重定向（与浏览器行为一致），
// Cookie 会自动在每跳之间流转，最终拿到 CASTGC + MOD_AUTH_CAS 等完整登录态。
func QRLoginFollow(ctx context.Context, redirectURL string) (*QRLoginCookies, error) {
	jar, _ := cookiejar.New(nil)
	// 跳转链上访问过的每个 URL：cookiejar.Cookies 返回的 Cookie 只填
	// Name/Value，Domain 恒为空串，域归属只能靠「用哪个 URL 去查」来还原。
	visited := []*url.URL{parseURL(redirectURL)}
	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 15 {
				return fmt.Errorf("重定向超过 15 次，终止跳转链")
			}
			visited = append(visited, req.URL)
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", redirectURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaApp)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	log.Printf("[qr-login] 跟随跳转链开始：%s", redirectURL)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[qr-login] 跳转链请求失败：%v（已访问 %v）", err, visitedURLHosts(visited))
		return nil, fmt.Errorf("跟随跳转链失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.Request != nil && resp.Request.URL != nil {
		visited = append(visited, resp.Request.URL)
	}
	log.Printf("[qr-login] 跳转链共 %d 跳，最终落点 %s（HTTP %d）", len(visited), resp.Request.URL, resp.StatusCode)
	// 中央域也要查一遍：CASTGC / uniauth 会话不在学校租户域上。
	visited = append(visited, parseURL("https://api.campushoy.com"), parseURL("https://uniauth.campusphere.net"))

	// 从 CookieJar 提取所有 Cookie。Go 的 cookiejar.Cookies(u) 只会填
	// Name/Value（Domain 恒为空），所以 Cookie 的域归属必须用
	// 「拿哪个 URL 去查」来还原：依次用跳转链上的每个 URL 查询，
	// 命中的 Cookie 就属于那个 URL 的主机。
	out := &QRLoginCookies{Cookies: map[string]string{}}
	schoolHosts := map[string]bool{}
	collectCookies := func(jar *cookiejar.Jar, urls []*url.URL) {
		for _, u := range urls {
			if u == nil {
				continue
			}
			host := u.Hostname()
			var names []string
			for _, c := range jar.Cookies(u) {
				names = append(names, c.Name)
				key := host + "|" + c.Name
				if _, ok := out.Cookies[key]; !ok {
					out.Cookies[key] = c.Value
				}
				// 学校域名判断：campusphere 租户且非中央域（与原实现口径一致）
				if strings.Contains(host, "campusphere.net") &&
					!strings.Contains(host, "api.campushoy.com") &&
					!strings.Contains(host, "uniauth.campusphere.net") {
					schoolHosts[host] = true
				}
			}
			if len(names) > 0 {
				log.Printf("[qr-login] 主机 %s 携带 Cookie：%v", host, names)
			}
		}
	}
	collectCookies(jar, visited)

	// 第一段链路的终点是 campushoy 的 authTransit 中转页：它本该在 App
	// WebView 里通过原生桥拿 cookie_token 再跳 /connect/oauth2/authorize，
	// 纯服务端走不了桥。但 uniauth 链路已经种下了 campushoy 账号会话
	// （MOD_AUTH_CAS=campushoy:…），带上它直接打标准 OAuth authorize，
	// 就能换到学校门户的业务会话——实测跳转链的 redirect_uri 里带着
	// 门户地址（…/iap/oauthCallback），这正是扫码登录真正的落点。
	transitHit := false
	if out.TenantHost == "" && resp.Request != nil && resp.Request.URL != nil {
		final := resp.Request.URL
		if strings.Contains(final.Path, "/h5/authTransit/") && final.Query().Get("redirect_uri") != "" {
			transitHit = true
			casCookie := ""
			for key, val := range out.Cookies {
				if strings.HasSuffix(key, "|MOD_AUTH_CAS") && strings.HasPrefix(val, "campushoy:") {
					casCookie = val
					break
				}
			}
			log.Printf("[qr-login] 命中 authTransit 中转页，改带 campushoy 会话直打 OAuth authorize（会话 %s…）", casCookie[:min(len(casCookie), 20)])
			if casCookie != "" {
				authURL := *final
				authURL.Scheme = "https"
				authURL.Host = "api.campushoy.com"
				authURL.Path = "/connect/oauth2/authorize"
				authorizeVisited := []*url.URL{&authURL}
				authorizeClient := &http.Client{
					Jar:     jar,
					Timeout: 30 * time.Second,
					Transport: &http.Transport{
						TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
					},
					CheckRedirect: func(req *http.Request, via []*http.Request) error {
						if len(via) >= 15 {
							return fmt.Errorf("重定向超过 15 次，终止跳转链")
						}
						authorizeVisited = append(authorizeVisited, req.URL)
						return nil
					},
				}
				authReq, aerr := http.NewRequestWithContext(ctx, http.MethodGet, authURL.String(), nil)
				if aerr == nil {
					authReq.Header.Set("User-Agent", uaApp)
					authReq.Header.Set("Cookie", "MOD_AUTH_CAS="+casCookie)
					authResp, aerr := authorizeClient.Do(authReq)
					if aerr != nil {
						log.Printf("[qr-login] OAuth authorize 请求失败：%v", aerr)
					} else {
						defer authResp.Body.Close()
						io.Copy(io.Discard, authResp.Body)
						if authResp.Request != nil && authResp.Request.URL != nil {
							authorizeVisited = append(authorizeVisited, authResp.Request.URL)
						}
						log.Printf("[qr-login] OAuth authorize 完成：最终落点 %s（HTTP %d）", authResp.Request.URL, authResp.StatusCode)
						collectCookies(jar, authorizeVisited)
					}
				}
			}
		}
	}
	if out.TenantHost == "" {
		log.Printf("[qr-login] 未识别学校域名：访问过的主机 %v，Cookie 明细 %v", visitedURLHosts(visited), out.Cookies)
	}

	// 提取学校 host
	for h := range schoolHosts {
		if out.TenantHost == "" || len(h) < len(out.TenantHost) {
			out.TenantHost = h
		}
	}
	// 提取 TGC（CASTGC）
	for key, val := range out.Cookies {
		if strings.HasSuffix(key, "|CASTGC") {
			out.TGC = val
			break
		}
	}

	if out.TenantHost == "" {
		if transitHit {
			return nil, fmt.Errorf("campushoy 新版扫码授权需要在 App 内完成换票（authTransit 中转页依赖 App 原生桥铸造 cookie_token），网页端无法完成；" +
				"请改用「短信验证码」登录")
		}
		return nil, fmt.Errorf("未能从跳转链 Cookie 中识别学校域名")
	}
	return out, nil
}

// QRLoginImageURL 返回二维码图片的 URL。
func QRLoginImageURL(qrId string) string {
	return fmt.Sprintf(qrBaseURL+"/connect/qrcode/image/%s", qrId)
}

// ---------- 内部工具 ----------

// visitedURLHosts 汇总已访问 URL 的主机名（日志排查用）。
func visitedURLHosts(urls []*url.URL) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u == nil {
			continue
		}
		out = append(out, u.Hostname())
	}
	return out
}

func parseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}

func httpDo(ctx context.Context, req *http.Request) (*http.Response, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// 与主链路一致的证书校验策略：只对管理页白名单里的主机跳过校验，
			// 不再对扫码链路整体关闭校验（这条链路上有 CASTGC / MOD_AUTH_CAS）。
			TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
		},
	}
	return client.Do(req)
}
