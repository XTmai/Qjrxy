// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// scanLoginUserAgent 是向学校门户要 OAuth 参数时使用的 UA。
//
// 必须用 iOS 今日校园客户端的标识：服务端按 UA 判断「是不是 App 自己发起的
// 换票请求」，用 Android okhttp 那套（casUserAgent）会被当成普通浏览器，
// 302 到登录页而不是下发 authorize 地址。
const scanLoginUserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) " +
	"AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 cpdaily/9.9.24"

// ScanLoginOAuth 是一所学校走「扫码换票」所需的 OAuth 三要素。
type ScanLoginOAuth struct {
	ClientID    string
	RedirectURI string
	State       string
}

// FetchOAuthClientID 向学校门户的 cpdailyLogin 端点要本校专属的 OAuth 参数。
//
// 背景：扫码换票最后一步是请求
//
//	api.campushoy.com/connect/oauth2/authorize?client_id=...&redirect_uri=...
//
// 而 client_id 是「每所学校一份」的 OAuth 应用注册号，写死任何一个值都只对
// 那所学校有效（实测 16896453639520707 只对三峡大学科技学院放行，其余 8 所
// 全部返回 error/500，与参数、路径、scope 都无关）。
//
// 这个值不用抓包也不用从 App 里找——学校门户自己有端点负责拼好它：
//
//	GET https://<host>/iap/cpdailyLogin?service=<业务页>
//	  → 302 https://api.campushoy.com/connect/oauth2/authorize
//	         ?response_type=code&client_id=<该校专属>&redirect_uri=<host>/iap/oauthCallback&state=<随机>
//
// 把 Location 解析出来即可。实测结果：
//
//	三峡大学科技学院 16896453639520707
//	韩山师范学院     16896435830199132
//	上海工程技术大学 16951737968849022
func FetchOAuthClientID(ctx context.Context, opts ClientOptions, host, service string) (*ScanLoginOAuth, error) {
	host = NormalizeHost(host)
	if host == "" {
		return nil, errors.New("缺少学校域名，无法换票")
	}
	// 诊断：扫码取 OAuth 参数这条链路的 UA 选择很微妙（iOS UA 才返回
	// client_id），出问题时必须能看到实际发出的头与那一跳 Location。
	ctx = WithFlow(ctx, FlowScanLogin)
	if strings.TrimSpace(service) == "" {
		service = "https://" + host + "/wec-counselor-attendance-apps/student/attendance/index.html"
	}

	target := "https://" + host + "/iap/cpdailyLogin?service=" + url.QueryEscape(service)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	// UA 决定 cpdailyLogin 走哪条分支，这点很关键：
	// 用 CAS 登录那套 Android okhttp UA 时，服务器判为「未登录」→ 302 到
	// /iap/login/mobile.html；换成 iOS 今日校园 UA 才直接下发 authorize 地址。
	// 实测（韩山师范学院）：casUserAgent → 登录页；iOS UA → client_id 正常返回。
	req.Header.Set("User-Agent", scanLoginUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh-Hans;q=0.9")

	// 必须手动跟随：我们要的是「带 client_id 的那一跳」的 Location，
	// 自动跟随会一路跑到学校首页，把参数丢掉。套诊断后这跳的
	// Location 与 set-cookie 都能在日志里核对。
	client := HTTPClient(opts.Label, 20*time.Second,
		&http.Transport{TLSClientConfig: tlsConfigFor(opts.InsecureHosts)},
		func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		})
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法获取该校扫码登录参数：%w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		// 未登录时可能直接返回登录页 200；也接受带 code 的 200 跳转页。
		return nil, fmt.Errorf("该校未返回扫码登录参数（HTTP %d），可能不支持扫码登录", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return nil, errors.New("该校未返回扫码登录参数（无跳转地址）")
	}
	u, err := url.Parse(loc)
	if err != nil {
		return nil, errors.New("该校返回的跳转地址无法解析")
	}
	if !strings.Contains(u.Path, "/connect/oauth2/authorize") {
		return nil, fmt.Errorf("该校登录跳转未指向授权端点（%s），可能不支持扫码登录", u.Path)
	}
	q := u.Query()
	out := &ScanLoginOAuth{
		ClientID:    strings.TrimSpace(q.Get("client_id")),
		RedirectURI: strings.TrimSpace(q.Get("redirect_uri")),
		State:       strings.TrimSpace(q.Get("state")),
	}
	if out.ClientID == "" || out.RedirectURI == "" {
		return nil, errors.New("该校授权跳转缺少 client_id 或 redirect_uri")
	}
	// redirect_uri 有可能是相对路径，补成绝对地址再交给前端。
	if ru, err := url.Parse(out.RedirectURI); err == nil && ru.Host == "" {
		base := &url.URL{Scheme: "https", Host: host}
		out.RedirectURI = base.ResolveReference(ru).String()
	}
	return out, nil
}
