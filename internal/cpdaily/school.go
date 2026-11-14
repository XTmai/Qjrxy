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
//
// 优先级（高 → 低）：
//  1. campusphere/cpdaily 托管域——查寝接口一定在这类域上；
//  2. 路径含 /wec-portal-mobile/ 的门户——辅导猫移动门户的标志路径。
//     学校自建部署时它在学校自有域名上（如厦门医学院 cat.xmmc.edu.cn），
//     而 ampUrl 常被填成学校的 ehall（办事大厅），上面没有辅导猫接口；
//  3. 第一个可解析的地址（兜底）。
//
// 实测（2026-11-14 租户接口）：厦门医学院与安徽城市管理职业学院的入口
// 都是「ampUrl=ehall + ampUrl2=辅导猫门户」结构，旧逻辑回退到 ehall，
// 导致厦门医学院的业务接口全部 404。
func hostFromEntry(entry *tenantEntry) string {
	best := ""
	bestScore := -1
	for _, amp := range []string{entry.AmpURL, entry.AmpURL2} {
		if strings.TrimSpace(amp) == "" {
			continue
		}
		u, err := url.Parse(amp)
		if err != nil || u.Host == "" {
			continue
		}
		score := 0
		if strings.Contains(amp, "campusphere") || strings.Contains(amp, "cpdaily") {
			score = 2 // 托管域：最高优先级
		} else if strings.Contains(u.Path, "wec-portal-mobile") {
			score = 1 // 学校自建的辅导猫门户
		}
		if score > bestScore {
			best, bestScore = u.Host, score
		}
		if score == 2 {
			break // 已是最高优先级，不必再看
		}
	}
	return best
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
			AppID    string `json:"appId"`
		} `json:"data"`
	}
	if err := httpGetJSON(ctx, tenantListURL, &list); err != nil {
		return nil, err
	}

	var matched *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		JoinType string `json:"joinType"`
		AppID    string `json:"appId"`
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
		AppID:    matched.AppID,
	}, nil
}

// LoginURL 返回统一身份认证的登录地址。
func (t *Tenant) LoginURL() string {
	base := strings.TrimRight(t.IDSURL, "/")
	return base + "/login?service=" + url.QueryEscape("https://"+t.Host+"/portal/login")
}
