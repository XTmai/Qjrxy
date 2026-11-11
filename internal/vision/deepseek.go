// Package vision 调用多模态大模型识别图片。
//
// 当前用途：今日校园的「以图判图」签到验证码——服务端下发一张场景图
// （例如「以下图片是什么」），需要看图给出答案。字符 OCR 干不了这个，
// 只有真正看懂图的模型可以。
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
	"strings"
	"time"
)

// Config 是识别服务的连接配置。
type Config struct {
	APIKey  string // DEEPSEEK_API_KEY
	BaseURL string // DEEPSEEK_BASE_URL，默认 https://api.deepseek.com
	Model   string // DEEPSEEK_VISION_MODEL，默认 deepseek-v4.1-flash
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
		cfg.Model = "deepseek-v4.1-flash"
	}
}

// Enabled 报告识别服务是否已配置。
func Enabled() bool { return strings.TrimSpace(cfg.APIKey) != "" }

// Current 返回当前识别服务配置的副本（server 层做掩码后展示）。
func Current() Config {
	return Config{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model}
}

// DescribeCaptcha 识别今日校园的「以图判图」验证码。
//
// 这类验证码下发一张场景图并提问（如「以下图片是什么」），答案是要
// 逐个填进 scenesImageCodes 的文本。这里让多模态模型直接看图作答，
// 输出约定为 JSON 数组；解析交给调用方（parseAnswers 做宽容处理）。
// 温度压到 0：同一张图两次识别必须给出同一个答案。
func DescribeCaptcha(ctx context.Context, imageMIME, imageB64 string) (string, error) {
	const prompt = `这是一道今日校园签到时弹出的图形验证码，形式通常是看图答题：
图中有一个问题（例如「以下图片是什么」「下图是什么场景」）和若干需要判定的内容。
请仔细观察图片，按题目的提问作答：
- 若要求说出图片内容，请用简短的中文名词回答（如「公交车」「食堂」「图书馆」）；
- 若是选择题，请回答对应选项的编号或选项文字。
只输出一个 JSON 字符串数组，数组元素按题目顺序排列，不要输出任何解释或多余字符。
示例：["公交车"] 或 ["图书馆","教学楼"]`
	return describeImage(ctx, imageMIME, imageB64, prompt)
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
		return "", fmt.Errorf("识别服务返回 HTTP %d：%s", resp.StatusCode, truncate(string(respBody), 200))
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
