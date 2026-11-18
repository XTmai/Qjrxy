// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

// Package vision 调用多模态大模型识别图片。
//
// 当前用途：今日校园的「以图判图」签到验证码。服务端下发 9 张候选图
// 和一个目标词（如「自行车」），需要在九宫格里挑出包含该物品的格子。
// 字符 OCR 干不了这个，只有真正看懂图的模型可以。
//
// DeepSeek 官方 API 是 OpenAI 兼容格式，图片以 data URL 内联在消息里。
package vision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Config 是识别服务的连接配置。
type Config struct {
	APIKey  string // DEEPSEEK_API_KEY
	BaseURL string // DEEPSEEK_BASE_URL，默认 https://api.deepseek.com
	Model   string // DEEPSEEK_VISION_MODEL，默认 deepseek-flash
}

var cfg Config

// Configure 注入识别服务配置；main 在启动时调用。
// 传空 BaseURL/Model 时用默认值。
func Configure(apiKey, baseURL, model string) {
	cfg = Config{APIKey: apiKey, BaseURL: baseURL, Model: model}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		// DeepSeek 当前的视觉模型名。名称随版本轮换，管理员设置页
		// 可以覆盖；调用返回 400 时错误信息会提示这一点。
		cfg.Model = "deepseek-flash"
	}
}

// Enabled 报告识别服务是否已配置。
func Enabled() bool { return strings.TrimSpace(cfg.APIKey) != "" }

// Current 返回当前识别服务配置的副本（server 层做掩码后展示）。
func Current() Config {
	return Config{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model}
}

// PickCaptchaCells 在九宫格拼图里找出包含 target 的格子。
//
// gridMIME/gridB64 是 composeGrid 的输出；target 是服务端下发的目标词，
// 为空时退化为「按图里的题目文字判断」。返回 1 基序号（1-9），升序去重。
// 温度压到 0：同一张图两次识别必须给同一组编号。
func PickCaptchaCells(ctx context.Context, gridMIME, gridB64, target string, total int) ([]int, error) {
	if total <= 0 {
		total = 9
	}
	goal := fmt.Sprintf("目标物是「%s」", strings.TrimSpace(target))
	if strings.TrimSpace(target) == "" {
		goal = "图片里写着题目，请按题目的要求判断"
	}
	prompt := fmt.Sprintf(`这是一张 3×3 的九宫格拼图，每一格是一张候选图片，来自签到验证码。
%s。
请找出所有包含目标物的格子。格子按从左到右、从上到下编号：
1 2 3
4 5 6
7 8 9
只输出 JSON 整数数组，例如 [2,5]，不要输出任何解释。
如果没有任何一格符合，输出 []。`, goal)
	answer, err := describeImage(ctx, gridMIME, gridB64, prompt)
	if err != nil {
		return nil, err
	}
	cells := parseCells(answer, total)
	return cells, nil
}

// parseCells 从模型回答里提取格子编号。容忍 ```json 围栏、前后缀文字、
// 字符串形式的数字，并按 total 过滤越界值。
func parseCells(answer string, total int) []int {
	answer = strings.TrimSpace(answer)
	answer = strings.TrimPrefix(answer, "```json")
	answer = strings.TrimPrefix(answer, "```")
	answer = strings.TrimSuffix(answer, "```")
	answer = strings.TrimSpace(answer)

	// 取第一个 [ 到最后一个 ]：模型偶尔会在数组前后加一句说明
	if i := strings.Index(answer, "["); i >= 0 {
		if j := strings.LastIndex(answer, "]"); j > i {
			answer = answer[i : j+1]
		}
	}

	var nums []int
	if err := json.Unmarshal([]byte(answer), &nums); err != nil {
		var strs []string
		if err2 := json.Unmarshal([]byte(answer), &strs); err2 == nil {
			for _, s := range strs {
				var n int
				if _, err3 := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err3 == nil {
					nums = append(nums, n)
				}
			}
		} else {
			// 最后兜底：从文本里抠出所有 1-9 的数字
			for _, m := range regexp.MustCompile(`\d+`).FindAllString(answer, -1) {
				var n int
				if _, err3 := fmt.Sscanf(m, "%d", &n); err3 == nil {
					nums = append(nums, n)
				}
			}
		}
	}

	seen := make(map[int]bool, len(nums))
	out := make([]int, 0, len(nums))
	for _, n := range nums {
		if n < 1 || n > total || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// describeImage 把图片交给多模态模型，返回模型的文本回答。
// imageMIME 如 "image/png"；imageB64 是不带 data: 前缀的 base64。
func describeImage(ctx context.Context, imageMIME, imageB64, prompt string) (string, error) {
	if !Enabled() {
		return "", errors.New("未配置识别服务（DEEPSEEK_API_KEY）")
	}
	body := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:" + imageMIME + ";base64," + imageB64,
						},
					},
					{"type": "text", "text": prompt},
				},
			},
		},
		// 验证码判图只要一个确定性的短答案，温度压到 0，
		// 避免同一张图两次识别给出不同选项。
		"temperature": 0,
		"stream":      false,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求识别服务失败：%w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		hint := ""
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
			hint = "（模型名或接口地址可能已过期，可在管理员设置里改成账号可用的视觉模型）"
		}
		return "", fmt.Errorf("识别服务返回 HTTP %d%s：%s", resp.StatusCode, hint, truncate(string(respBody), 200))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("解析识别响应失败：%w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("识别服务报错：%s", out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("识别服务未返回内容")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// AnswerFromEnv 供测试注入环境变量读取的钩子。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
