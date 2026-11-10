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
	appSMSHost   = "https://mobile.campushoy.com"
	appSMSKey    = "fafUY9659kyengTR"
	appSMSIVHex  = "01020304050607080901020304050607"
	appSMSVerTag = "first_v4"
)

// AppLoginResult 是手机号登录成功后的关键信息。
type AppLoginResult struct {
	SessionToken string `json:"sessionToken"`
	TGC          string `json:"tgc"`
	OpenID       string `json:"openId"` // 学号
	TenantID     string `json:"tenantId"`
	TenantName   string `json:"tenantName"`
	Name         string `json:"name"`
	Status       string `json:"status"`
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

// AppMobileLogin 用手机号 + 验证码登录今日校园账号，返回学校 CAS 票与会话。
func AppMobileLogin(ctx context.Context, device DeviceProfile, tenantID, phone, code string) (*AppLoginResult, error) {
	phone = strings.TrimSpace(phone)
	code = strings.TrimSpace(code)
	if phone == "" || code == "" {
		return nil, errors.New("手机号或验证码为空")
	}
	payload, _ := json.Marshal(map[string]string{
		"d": code,
		"e": "CPDAILY",
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
		return nil, errors.New("登录未返回有效凭证")
	}
	return &res, nil
}

// iapLoginURL 构造换票入口。
//
// service 是 CAS 的必需参数：IAP 需要知道把票据签给哪个业务域，
// 缺了它即使 TGC 有效也只会 302 到登录页。
func iapLoginURL(host string) string {
	service := "https://" + host + "/portal/login"
	return "https://" + host + "/iap/login?service=" + url.QueryEscape(service)
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
	if tgc == "" || host == "" {
		return errors.New("缺少换票参数")
	}
	// TGC 头是 tgc 经 DES-CBC（XCE927== / IV 0102030405060708）加密后的 base64，
	// 直接塞明文 CAS 票服务端不认，会跳回 portal/login 形成重定向循环。
	tgcHeader, err := DESEncrypt(tgc, desKeyToken)
	if err != nil {
		return fmt.Errorf("构造换票凭证失败：%w", err)
	}
	target := iapLoginURL(host)

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

	for hop := 0; hop < 6; hop++ {
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

		if _, ok := client.Cookies()["MOD_AUTH_CAS"]; ok {
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

	if _, ok := client.Cookies()["MOD_AUTH_CAS"]; !ok {
		return errors.New("登录已通过，但未拿到业务会话（MOD_AUTH_CAS），请重试")
	}
	return nil
}
