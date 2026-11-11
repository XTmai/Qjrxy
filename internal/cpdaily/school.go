package cpdaily

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tenantListURL = "https://mobile.campushoy.com/v6/config/guest/tenant/list"
	tenantInfoURL = "https://mobile.campushoy.com/v6/config/guest/tenant/info"
)

// httpGetJSON 发起一个不依赖会话的 GET 请求。
//
// 证书校验沿用全局 TLS 策略：默认严格校验，只有 QINGJIN_TLS_INSECURE_HOSTS
// 里显式列出的主机才跳过。学校列表与入口地址直接决定后续登录发往哪里，
// 这条链路被劫持等于密码被劫持，不能再无条件关闭校验。
func httpGetJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) cpdaily/"+AppVersion)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		TLSClientConfig: tlsConfigFor(TLSInsecureHosts()),
	}}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求学校列表失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求学校列表失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ListSchools 拉取全部学校（可缓存）。
func ListSchools(ctx context.Context) ([]School, error) {
	var payload struct {
		Data []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			JoinType string `json:"joinType"`
		} `json:"data"`
	}
	if err := httpGetJSON(ctx, tenantListURL, &payload); err != nil {
		return nil, err
	}
	out := make([]School, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.JoinType == "NONE" {
			continue
		}
		out = append(out, School{ID: item.ID, Name: item.Name})
	}
	return out, nil
}

// tenantEntry 是租户信息接口返回的入口字段（只取用得到的部分）。
type tenantEntry struct {
	AmpURL    string `json:"ampUrl"`
	AmpURL2   string `json:"ampUrl2"`
	IDSURL    string `json:"idsUrl"`
	JoinType  string `json:"joinType"`
	TenantID  string `json:"id"`
	LcukeyURL string `json:"lcukeyUrl"`
}

// fetchTenantEntry 按租户 ID 拉取入口信息。
func fetchTenantEntry(ctx context.Context, tenantID string) (*tenantEntry, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("缺少租户 ID")
	}
	var info struct {
		Data []tenantEntry `json:"data"`
	}
	if err := httpGetJSON(ctx, tenantInfoURL+"?ids="+url.QueryEscape(tenantID), &info); err != nil {
		return nil, err
	}
	if len(info.Data) == 0 {
		return nil, fmt.Errorf("获取学校入口信息失败，请稍后重试")
	}
	return &info.Data[0], nil
}

// hostFromEntry 从 ampUrl/ampUrl2 里挑出门户域名。
// 优先 campusphere/cpdaily 域名，其次第一个能解析的地址。
func hostFromEntry(entry *tenantEntry) string {
	host := ""
	for _, amp := range []string{entry.AmpURL, entry.AmpURL2} {
		if amp == "" {
			continue
		}
		u, err := url.Parse(amp)
		if err != nil || u.Host == "" {
			continue
		}
		isCampus := strings.Contains(amp, "campusphere") || strings.Contains(amp, "cpdaily")
		if isCampus || host == "" {
			host = u.Host
		}
		if isCampus {
			break
		}
	}
	return host
}

// NormalizeHost 把用户可能填进来的各种形式（带 scheme / 带路径 / 前后空格）
// 统一成纯域名。换票地址是 "https://" + host 拼出来的，带 scheme 会拼成
// https://https://… 直接请求失败。
func NormalizeHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// 先剥 scheme 再截路径：反过来的话 "https://host" 会被 "/" 截成 "https:"。
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return ""
	}
	if u, err := url.Parse("https://" + s); err == nil && u.Host != "" {
		return u.Host
	}
	return ""
}

// ResolveTenantByID 用租户 ID 解析入口。
//
// 短信登录的响应自带 tenantId，它才是这次登录真正落在哪个租户——比账号上
// 久存的 tenantId 或用户手填的学校名都可信。部分学校「缺少换票参数」就是
// 因为账号没存 host，而按学校名匹配不到租户。
func ResolveTenantByID(ctx context.Context, tenantID string) (*Tenant, error) {
	entry, err := fetchTenantEntry(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	host := hostFromEntry(entry)
	if host == "" {
		return nil, fmt.Errorf("该校门户地址解析失败（学校未提供入口域名），请联系开发者补充支持")
	}
	ids := entry.IDSURL
	if ids == "" {
		ids = "https://" + host + "/iap"
	}
	return &Tenant{
		ID:       tenantID,
		Host:     host,
		IDSURL:   ids,
		JoinType: entry.JoinType,
		AmpURL:   entry.AmpURL,
	}, nil
}

// ResolveTenant 按学校名称解析租户与登录入口。
func ResolveTenant(ctx context.Context, schoolName string) (*Tenant, error) {
	schoolName = strings.TrimSpace(schoolName)
	if schoolName == "" {
		return nil, fmt.Errorf("请填写学校全称")
	}
	var list struct {
		Data []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			JoinType string `json:"joinType"`
		} `json:"data"`
	}
	if err := httpGetJSON(ctx, tenantListURL, &list); err != nil {
		return nil, err
	}

	var matched *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		JoinType string `json:"joinType"`
	}
	for i := range list.Data {
		if list.Data[i].Name == schoolName {
			matched = &list.Data[i]
			break
		}
	}
	if matched == nil {
		for i := range list.Data {
			if strings.Contains(list.Data[i].Name, schoolName) {
				matched = &list.Data[i]
				break
			}
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("没有找到「%s」，请填写今日校园中的学校全称", schoolName)
	}
	if matched.JoinType == "NONE" {
		return nil, fmt.Errorf("「%s」未开通今日校园", matched.Name)
	}

	entry, err := fetchTenantEntry(ctx, matched.ID)
	if err != nil {
		return nil, err
	}
	host := hostFromEntry(entry)
	if host == "" {
		return nil, fmt.Errorf("该校门户地址解析失败（学校未提供入口域名），请联系开发者补充支持")
	}

	ids := entry.IDSURL
	if ids == "" {
		ids = "https://" + host + "/iap"
	}

	return &Tenant{
		ID:       matched.ID,
		Name:     matched.Name,
		JoinType: matched.JoinType,
		Host:     host,
		IDSURL:   ids,
		AmpURL:   entry.AmpURL,
	}, nil
}

// LoginURL 返回统一身份认证的登录地址。
func (t *Tenant) LoginURL() string {
	base := strings.TrimRight(t.IDSURL, "/")
	return base + "/login?service=" + url.QueryEscape("https://"+t.Host+"/portal/login")
}
