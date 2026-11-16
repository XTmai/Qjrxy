package cpdaily

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// App 短信验证码登录协议（campushoy 官方账号体系）。
//
// 与学校 CAS 的短信无关：这里认证的是「今日校园账号绑定的手机号」，
// 登录成功后服务端返回学校 CAS 票（tgc），再用它换取业务域会话。
// 因此只要手机号绑定了学校账号，所有学校通用。
const (
	appSMSKey    = "fafUY9659kyengTR"
	appSMSIVHex  = "01020304050607080901020304050607"
	appSMSVerTag = "first_v4"
)

// appSMSHost 是账号体系域名；变量形式便于单测用 httptest 替换。
var appSMSHost = "https://mobile.campushoy.com"

// AppLoginResult 是手机号登录成功后的关键信息。
type AppLoginResult struct {
	SessionToken string `json:"sessionToken"`
	TGC          string `json:"tgc"`
	OpenID       string `json:"openId"` // 学号
	TenantID     string `json:"tenantId"`
	TenantName   string `json:"tenantName"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	// TGCStatus 只在部分学校出现；不是 tgcValidate 时说明票据还要二次校验。
	TGCStatus string `json:"tgcStatus"`
}

// appSMSEncrypt 加密登录体：AES-128-CBC + PKCS7，输出标准 base64。
func appSMSEncrypt(plain string) (string, error) {
	iv, err := hex.DecodeString(appSMSIVHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(appSMSKey))
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plain), block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

// appSMSDecrypt 解密响应体。
func appSMSDecrypt(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, err
	}
	iv, err := hex.DecodeString(appSMSIVHex)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher([]byte(appSMSKey))
	if err != nil {
		return nil, err
	}
	if len(raw)%block.BlockSize() != 0 || len(raw) == 0 {
		return nil, errors.New("响应长度异常")
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, raw)
	pad := int(out[len(out)-1])
	if pad < 1 || pad > block.BlockSize() {
		return out, nil
	}
	return out[:len(out)-pad], nil
}

func pkcs7Pad(data []byte, size int) []byte {
	pad := size - len(data)%size
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

// appSMSHeaders 构造 App 登录接口需要的设备头。
func appSMSHeaders(device DeviceProfile, tenantID string) map[string]string {
	model := device.DeviceModel
	if model == "" {
		model = "23127PN0CC"
	}
	systemVersion := device.SystemVersion
	if systemVersion == "" {
		systemVersion = "14"
	}
	platform := strings.ToLower(device.Platform)
	isIOS := platform == "ios"
	appVersion := device.AppVersion
	if appVersion == "" {
		appVersion = AppVersion
	}
	systemName := "android"
	if isIOS {
		systemName = "iOS"
	}
	deviceInfo := map[string]any{
		"systemName":    systemName,
		"systemVersion": systemVersion,
		"model":         model,
		"deviceId":      device.DeviceID,
		"appVersion":    appVersion,
	}
	if isIOS {
		deviceInfo["cpdailyVersion"] = appVersion
	}
	deviceJSON, _ := json.Marshal(deviceInfo)
	infoEnc, _ := DESEncrypt(string(deviceJSON), desKeyInfo)

	h := map[string]string{
		"User-Agent":        device.UserAgent(),
		"Content-Type":      "application/json; charset=utf-8",
		"Accept":            "application/json, text/plain, */*",
		"Accept-Language":   "zh-CN,zh-Hans;q=0.9",
		"CpdailyStandAlone": "0",
		"CpdailyClientType": "CPDAILY",
		"CacheTimeValue":    "0",
		"tenantId":          tenantID,
		"CpdailyInfo":       infoEnc,
		"clientType":        "cpdaily_student",
	}
	if isIOS {
		h["deviceType"] = "2"
		if device.WisDeviceID != "" {
			h["wisDeviceId"] = device.WisDeviceID
		}
	} else {
		h["deviceType"] = "1"
	}
	h["Cookie"] = fmt.Sprintf("clientType=cpdaily_student; standAlone=0; tenantId=%s", tenantID)
	return h
}

// appSMSPost 发送加密请求并解析外层 JSON。
func appSMSPost(ctx context.Context, path string, plain string, headers map[string]string) (map[string]json.RawMessage, error) {
	enc, err := appSMSEncrypt(plain)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{"a": enc, "b": appSMSVerTag})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, appSMSHost+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求今日校园账号服务失败：%w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("账号服务响应异常（HTTP %d）", resp.StatusCode)
	}
	var errCode int
	var errMsg string
	_ = json.Unmarshal(out["errCode"], &errCode)
	_ = json.Unmarshal(out["errMsg"], &errMsg)
	if errCode != 0 {
		if errMsg == "" {
			errMsg = fmt.Sprintf("code=%d", errCode)
		}
		return nil, fmt.Errorf("登录被拒绝：%s", errMsg)
	}
	return out, nil
}

// SendAppSMSCode 向绑定手机号发送验证码。
func SendAppSMSCode(ctx context.Context, device DeviceProfile, tenantID, phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return errors.New("请填写手机号")
	}
	_, err := appSMSPost(ctx,
		"/app/auth/authentication/mobile/messageCode/v-8222",
		phone, appSMSHeaders(device, tenantID))
	return err
}

// SMSChannel* 是短信登录的验证码渠道值（载荷字段 e）。
const (
	SMSChannelCpdaily = "CPDAILY" // 今日校园渠道下发的验证码
	SMSChannelMob     = "MOB"     // MobTech 渠道下发的验证码
)

// AppMobileLogin 用手机号 + 验证码登录今日校园账号，返回学校 CAS 票与会话。
//
// channel 是验证码的下发渠道：IPA 逆向（9.9.22）证实该字段在
// "CPDAILY"（今日校园渠道发码）与 "MOB"（MobTech 渠道发码）之间切换，
// 由登录页的 isMobSMSLogin 决定。渠道必须与收到的验证码匹配，
// 用错渠道服务端会拒——NOTCLOUD 学校的验证码可能走 MOB 渠道。
func AppMobileLogin(ctx context.Context, device DeviceProfile, tenantID, phone, code, channel string) (*AppLoginResult, error) {
	phone = strings.TrimSpace(phone)
	code = strings.TrimSpace(code)
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = SMSChannelCpdaily
	}
	if phone == "" || code == "" {
		return nil, errors.New("手机号或验证码为空")
	}
	payload, _ := json.Marshal(map[string]string{
		"d": code,
		"e": channel,
		"c": phone,
		"f": "86",
	})
	out, err := appSMSPost(ctx,
		"/app/auth/authentication/mobileLogin/v-8222",
		string(payload), appSMSHeaders(device, tenantID))
	if err != nil {
		return nil, err
	}
	var encrypted string
	if err := json.Unmarshal(out["data"], &encrypted); err != nil {
		return nil, errors.New("登录响应缺少数据")
	}
	dec, err := appSMSDecrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("解析登录响应失败：%w", err)
	}
	var res AppLoginResult
	if err := json.Unmarshal(dec, &res); err != nil {
		return nil, fmt.Errorf("解析登录响应失败：%w", err)
	}
	if res.TGC == "" && res.SessionToken == "" {
		// 把服务端的状态码带上：不同学校的失败形态不一样（选租户、待绑定、
		// 票据二次校验），没有这两个字段只能看到一句「没凭证」，无从排查。
		return nil, fmt.Errorf("登录未返回有效凭证（status=%s, tgcStatus=%s）",
			orDash(res.Status), orDash(res.TGCStatus))
	}
	return &res, nil
}

// RefreshTGCBySession 用账号体系的 sessionToken 换取/刷新学校 CAS 票。
//
// 背景：部分学校短信登录只返回 sessionToken、不返回 tgc（CASTGC），
// 业务票（MOD_AUTH_CAS）过期后就没有可续期的票据。App 自己会带
// sessionToken 调 validation 接口拿一张新 tgc，这里复刻该流程。
//
// tgc 为空或已过期都可以传空串；服务端会基于 sessionToken 重新签发。
func RefreshTGCBySession(ctx context.Context, device DeviceProfile, tenantID, sessionToken, tgc string) (*AppLoginResult, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil, errors.New("缺少 sessionToken，无法刷新 CAS 票")
	}
	body := map[string]string{"tgc": "", "idsToken": ""}
	if tgc = strings.TrimSpace(tgc); tgc != "" {
		if enc, err := DESEncrypt(tgc, desKeyToken); err == nil {
			body["tgc"] = enc
		}
	}
	payload, _ := json.Marshal(body)

	h := appSMSHeaders(device, tenantID)
	h["Cookie"] = fmt.Sprintf("clientType=cpdaily_student; sessionToken=%s; standAlone=0; tenantId=%s",
		sessionToken, tenantID)
	h["sessionTokenKey"] = sessionToken

	// 注意：这个接口收的是明文 JSON（不是 appSMSPost 的 {a,b} 加密信封）。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		appSMSHost+"/v6/auth/authentication/new/validation/v-9012",
		strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
	}}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 CAS 票服务失败：%w", err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var outer struct {
		ErrCode int             `json:"errCode"`
		ErrMsg  string          `json:"errMsg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &outer); err != nil {
		return nil, fmt.Errorf("CAS 票服务响应异常（HTTP %d）", resp.StatusCode)
	}
	if outer.ErrCode != 0 {
		msg := outer.ErrMsg
		if msg == "" {
			msg = fmt.Sprintf("code=%d", outer.ErrCode)
		}
		return nil, fmt.Errorf("刷新 CAS 票被拒绝：%s", msg)
	}
	raw := outer.Data
	// 该接口通常返回明文 data 对象；兼容被加密返回的旧版本。
	var res AppLoginResult
	if err := json.Unmarshal(raw, &res); err != nil {
		var encrypted string
		if json.Unmarshal(raw, &encrypted) != nil {
			return nil, errors.New("解析 CAS 票响应失败")
		}
		dec, derr := appSMSDecrypt(encrypted)
		if derr != nil {
			return nil, fmt.Errorf("解析 CAS 票响应失败：%w", derr)
		}
		if err := json.Unmarshal(dec, &res); err != nil {
			return nil, fmt.Errorf("解析 CAS 票响应失败：%w", err)
		}
	}
	if res.TGC == "" && res.SessionToken == "" {
		return nil, fmt.Errorf("账号体系未返回票据（status=%s, tgcStatus=%s）",
			orDash(res.Status), orDash(res.TGCStatus))
	}
	return &res, nil
}

// EnsureAppLoginTGC 补齐短信登录首次未下发的 CAS 票。
//
// 部分 NOTCLOUD 学校的 mobileLogin 只返回 sessionToken。App 会紧接着用
// validation/v-9012 补签 tgc；若省略这一步，首次登录只能拿到无法访问业务
// 接口的账号会话，必须等后续探测/续期才可能恢复。
//
// 已有 tgc 或没有 sessionToken 时不发请求。补签响应只合并会话与票据状态；
// tenantId 不在这里覆盖，NOTCLOUD 的学校门户/租户处理仍由调用方负责。
func EnsureAppLoginTGC(ctx context.Context, device DeviceProfile, tenantID string, result *AppLoginResult) error {
	if result == nil {
		return errors.New("短信登录结果为空")
	}
	if strings.TrimSpace(result.TGC) != "" || strings.TrimSpace(result.SessionToken) == "" {
		return nil
	}

	refreshed, err := RefreshTGCBySession(ctx, device, tenantID, result.SessionToken, "")
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(refreshed.SessionToken); token != "" {
		result.SessionToken = token
	}
	if refreshed.Status != "" {
		result.Status = refreshed.Status
	}
	if refreshed.TGCStatus != "" {
		result.TGCStatus = refreshed.TGCStatus
	}
	result.TGC = strings.TrimSpace(refreshed.TGC)
	if result.TGC == "" {
		return fmt.Errorf("账号体系补签后仍未返回 CAS 票（status=%s, tgcStatus=%s）",
			orDash(result.Status), orDash(result.TGCStatus))
	}
	return nil
}

// orDash 空串显示为 "-"，便于日志与错误信息阅读。
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// iapLoginURL 构造换票入口。
//
// service 是 CAS 的必需参数：IAP 需要知道把票据签给哪个业务域，
// 缺了它即使 TGC 有效也只会 302 到登录页。
func iapLoginURL(host string) string {
	service := "https://" + host + "/portal/login"
	return "https://" + host + "/iap/login?service=" + url.QueryEscape(service)
}

// freshMODAuthCAS 判断 MOD_AUTH_CAS 是否是本次换票新拿到的。
// 进入前已有的旧票（通常是过期票）不能算成功。
func freshMODAuthCAS(initial string, cookies map[string]string) bool {
	cur := strings.TrimSpace(cookies["MOD_AUTH_CAS"])
	return cur != "" && cur != strings.TrimSpace(initial)
}

// ExchangeTGC 用学校 CAS 票（tgc）换取业务域会话（MOD_AUTH_CAS）。
//
// 两个容易踩的点：
//  1. service 是 CAS 的必需参数。IAP 需要知道把票据签给哪个业务域，
//     不带 service 时即使 TGC 有效也只会 302 到 mobile.html 登录页。
//  2. MOD_AUTH_CAS 往往种在 ticket 校验之后的那一跳，只跟一次会漏。
//
// 设备头（CpdailyInfo / wisDeviceId）一并带上，兼容要求校验设备的部署。
func ExchangeTGC(ctx context.Context, client *Client, host, tgc string, device DeviceProfile, tenantID string) error {
	tgc = strings.TrimSpace(tgc)
	host = NormalizeHost(host)
	// 两个参数缺失的原因完全不同，分开报：混成一句「缺少换票参数」时，
	// 用户和我们都无法判断是该换学校名，还是该校根本不发 CAS 票。
	if host == "" {
		return errors.New("未识别到学校门户域名（host），请填写今日校园里的学校全称后重试")
	}
	if tgc == "" {
		return errors.New("该校登录未下发换票票据（tgc），无法换取业务会话；请改用账密登录或网页登录")
	}
	// TGC 头是 tgc 经 DES-CBC（XCE927== / IV 0102030405060708）加密后的 base64，
	// 直接塞明文 CAS 票服务端不认，会跳回 portal/login 形成重定向循环。
	tgcHeader, err := DESEncrypt(tgc, desKeyToken)
	if err != nil {
		return fmt.Errorf("构造换票凭证失败：%w", err)
	}
	target := iapLoginURL(host)
	originSite := registrableDomain(host)

	// 记录进入前已有的 MOD_AUTH_CAS：它通常是过期票。若把它当成换票成功，
	// 调用方会以为会话已恢复，实际后续业务请求仍然 401。
	// 只有拿到「不同的新票」才算成功。
	initialMOD := strings.TrimSpace(client.Cookies()["MOD_AUTH_CAS"])
	newSession := func() bool { return freshMODAuthCAS(initialMOD, client.Cookies()) }

	jar := map[string]string{"AUTHTGC": tgc, "CASTGC": tgc}
	cookieHeader := func() string {
		parts := make([]string, 0, len(jar))
		for k, v := range jar {
			parts = append(parts, k+"="+v)
		}
		return strings.Join(parts, "; ")
	}

	noRedirect := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
		},
		// 必须显式禁止自动跟随：未认证时业务域会与门户互相跳转，
		// 自动跟随会一路跑到 10 次重定向上限。
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	offSite := ""
	for hop := 0; hop < 6; hop++ {
		// 逐跳前先校验目标是否还在本校站点内：换票链路带着明文
		// CASTGC/AUTHTGC 和加密 TGC 头，跳到中央域或第三方主机等于把
		// CAS 票送出去。跳出站点时不带凭据硬闯，而是停下——
		// 若此时已换到新票就正常返回，否则在循环外报错说明原因。
		if u, perr := url.Parse(target); perr != nil || !sameSite(u.Hostname(), originSite) {
			offSite = target
			break
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", casUserAgent)
		req.Header.Set("TGC", tgcHeader)
		req.Header.Set("Cookie", cookieHeader())
		// 复用短信登录的设备头，但 Cookie 要换成 CAS 票
		for k, v := range appSMSHeaders(device, tenantID) {
			switch k {
			case "Cookie", "Content-Type":
				continue
			}
			req.Header.Set(k, v)
		}

		resp, err := noRedirect.Do(req)
		if err != nil {
			return fmt.Errorf("换取业务会话失败：%w", err)
		}
		client.absorb(resp)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		for _, ck := range resp.Cookies() {
			if ck.Name != "" && ck.MaxAge >= 0 {
				jar[ck.Name] = ck.Value
			}
		}
		loc := resp.Header.Get("Location")
		resp.Body.Close()

		if newSession() {
			return nil
		}
		if loc == "" {
			break
		}
		if u, perr := url.Parse(loc); perr == nil && !u.IsAbs() {
			if base, berr := url.Parse(target); berr == nil {
				loc = base.ResolveReference(u).String()
			}
		}
		target = loc
	}

	if !newSession() {
		if offSite != "" {
			return fmt.Errorf("换票链路跳出了学校站点（%s），已中止以免票据外泄，且未拿到新的业务会话", offSite)
		}
		return errors.New("登录已通过，但未拿到新的业务会话（MOD_AUTH_CAS），请重试")
	}
	return nil
}

// registrableDomain 取主机名的可注册域（xx.edu.cn 类二级域取三段，
// 其余取两段），用于判断两台主机是否属于同一站点。
func registrableDomain(host string) string {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	tail := strings.Join(parts[len(parts)-2:], ".")
	if tail == "edu.cn" || tail == "gov.cn" || tail == "com.cn" || tail == "net.cn" || tail == "org.cn" {
		if len(parts) >= 3 {
			return strings.Join(parts[len(parts)-3:], ".")
		}
	}
	return tail
}

// sameSite 判断两台主机是否属于同一站点（可注册域相同）。
// originSite 为空时视为不确定，一律放行（保持旧行为，不误伤）。
func sameSite(host, originSite string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || originSite == "" {
		return true
	}
	return registrableDomain(host) == originSite
}
