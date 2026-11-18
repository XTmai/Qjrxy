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

	"github.com/PuerkitoBio/goquery"
)

// SelfCASProfile 描述一所「学校自建统一身份认证（CAS）」的登录要素。
//
// 这类学校在今日校园租户表里通常标记为 CLOUD，但 /iap/login 会 302 到
// 学校自己的 CAS（如 my.nut.edu.cn/cas/login）。其密码加密规则由学校
// 自己的 security.js 决定（南昌理工为「反转 + RSAUtils 十六进制」），
// 通用 IAP 的 {rsa} 格式会被判为密码错误。短信登录不经过密码，因此
// 对这类学校是更稳的路径。
type SelfCASProfile struct {
	// LoginURL 是带 service 参数的完整登录页地址（302 目标）。
	LoginURL string
	// Base 是认证服务根，例如 https://my.nut.edu.cn/cas。
	Base string
}

// DetectSelfHostedCAS 探测 /iap/login 是否跳转到学校自建 CAS。
// 不是自建 CAS 时返回 (nil, info, nil)，info 里带实际入口与跳转位置，便于排查。
func DetectSelfHostedCAS(ctx context.Context, client *Client, tenant *Tenant) (*SelfCASProfile, string, error) {
	entry := serviceBase(tenant.IDSURL, "iap") + "/login"
	resp, err := client.doNoRedirect(ctx, http.MethodGet, entry, map[string]string{
		"User-Agent": casUserAgent,
	}, nil)
	if err != nil {
		return nil, entry, err
	}
	defer resp.Body.Close()
	info := fmt.Sprintf("入口 %s → HTTP %d", entry, resp.StatusCode)
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return nil, info, nil
	}
	loc := strings.TrimSpace(resp.Header.Get("Location"))
	info += " → " + loc
	if loc == "" {
		return nil, info, nil
	}
	u, err := url.Parse(loc)
	if err != nil || u.Host == "" {
		return nil, info, nil
	}
	host := strings.ToLower(u.Host)
	// 仍在本校 campusphere 域内 → 是标准 IAP，不是自建 CAS。
	// 注意：tenant.Host 可能为空（老数据/解析失败），空串会让 Contains 恒真，
	// 必须显式判空，否则所有学校都会被误判成「标准认证」。
	th := strings.ToLower(strings.TrimSpace(tenant.Host))
	if strings.Contains(host, "campusphere.net") || (th != "" && strings.Contains(host, th)) {
		return nil, info, nil
	}
	base := u.Scheme + "://" + u.Host + strings.TrimSuffix(u.Path, "/login")
	base = strings.TrimRight(base, "/")
	if base == "" {
		return nil, info, nil
	}
	return &SelfCASProfile{LoginURL: loc, Base: base}, info, nil
}

// OpenSelfCASLogin 打开 CAS 登录页，解析表单隐藏字段（execution / _eventId 等），
// 并返回一个可直接提交的 LoginState。此流程不涉及密码。
func OpenSelfCASLogin(ctx context.Context, client *Client, profile *SelfCASProfile) (*LoginState, error) {
	resp, err := client.do(ctx, http.MethodGet, profile.LoginURL, map[string]string{
		"User-Agent": casUserAgent,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("打开学校统一身份认证失败：%w", err)
	}
	defer resp.Body.Close()
	page, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(page)))
	if err != nil {
		return nil, fmt.Errorf("解析认证页失败：%w", err)
	}

	form := doc.Find("#fm1").First()
	if form.Length() == 0 {
		// 部分学校用 pwdFromId / casLoginForm / loginFromId，沿用 CAS 分支的选择器
		for _, sel := range []string{"#pwdFromId", "#casLoginForm", "#loginFromId"} {
			if doc.Find(sel).Length() > 0 {
				form = doc.Find(sel).First()
				break
			}
		}
	}
	if form.Length() == 0 {
		if strings.Contains(string(page), "验证码") || strings.Contains(string(page), "短信") {
			return nil, errors.New("未识别到登录表单（页面结构可能已改版），暂时无法短信登录")
		}
		return nil, errors.New("该校认证页没有可识别的登录表单")
	}

	submitURL := profile.LoginURL
	if action, ok := form.Attr("action"); ok && strings.TrimSpace(action) != "" {
		if abs, err := resolveURL(profile.LoginURL, action); err == nil {
			submitURL = abs
		}
	}

	params := map[string]string{}
	form.Find("input").Each(func(_ int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok || name == "" {
			return
		}
		switch name {
		case "password", "mobileCode", "authcode", "captcha":
			// 这些由本次登录动态填写
			return
		}
		value, _ := s.Attr("value")
		params[name] = value
	})
	// _eventId 缺失时补上，Apereo CAS 用它区分提交动作
	if _, ok := params["_eventId"]; !ok {
		params["_eventId"] = "submit"
	}

	state := &LoginState{
		Tenant:    nil,
		LoginURL:  submitURL,
		Params:    params,
		Cookies:   client.Cookies(),
		CreatedAt: time.Now(),
	}
	return state, nil
}

// resolveURL 把相对 action 解析成绝对地址。
func resolveURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}
