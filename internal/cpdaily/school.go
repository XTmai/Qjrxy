package cpdaily

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tlsConfig 返回允许跳过证书校验的配置。
// 今日校园部分学校入口证书链不完整，官方 App 同样忽略，这里保持一致。
func insecureTLSConfig(insecure bool) *tls.Config {
	return &tls.Config{InsecureSkipVerify: insecure, MinVersion: tls.VersionTLS12}
}

const (
	tenantListURL = "https://mobile.campushoy.com/v6/config/guest/tenant/list"
	tenantInfoURL = "https://mobile.campushoy.com/v6/config/guest/tenant/info"
)

// httpGetJSON 发起一个不依赖会话的 GET 请求。
func httpGetJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) cpdaily/"+AppVersion)
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
		TLSClientConfig: insecureTLSConfig(true),
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

	infoURL := tenantInfoURL + "?ids=" + url.QueryEscape(matched.ID)
	var info struct {
		Data []struct {
			AmpURL    string `json:"ampUrl"`
			AmpURL2   string `json:"ampUrl2"`
			IDSURL    string `json:"idsUrl"`
			JoinType  string `json:"joinType"`
			TenantID  string `json:"id"`
			LcukeyURL string `json:"lcukeyUrl"`
		} `json:"data"`
	}
	if err := httpGetJSON(ctx, infoURL, &info); err != nil {
		return nil, err
	}
	if len(info.Data) == 0 {
		return nil, fmt.Errorf("获取学校入口信息失败，请稍后重试")
	}
	entry := info.Data[0]

	host := ""
	for _, amp := range []string{entry.AmpURL, entry.AmpURL2} {
		if amp == "" {
			continue
		}
		if u, err := url.Parse(amp); err == nil && u.Host != "" {
			if strings.Contains(amp, "campusphere") || strings.Contains(amp, "cpdaily") || host == "" {
				host = u.Host
			}
			if strings.Contains(amp, "campusphere") || strings.Contains(amp, "cpdaily") {
				break
			}
		}
	}
	if host == "" {
		return nil, fmt.Errorf("该校暂不支持自动解析入口，请手动填写 host")
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
