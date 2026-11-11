package cpdaily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"qjrxy/internal/vision"
)

// 「以图判图」验证码：部分学校的签到/查寝在提交前要求通过一个
// 场景验证码（下发一张图，问「以下图片是什么」），通过后签发一张
// 一次性的 ticket，随提交体一起走 AES + sign。缺 ticket 的提交会被
// 服务端以「验证码验签失败」(2210060000) 拒绝。
//
// 完整链路（对照 ruoli-sign-optimization 的实现还原）：
//  1. checkValidation —— 问服务端要不要验证码，要的话拿到 accountKey 等参数
//  2. captcha/create/scenesImage —— 拿场景图与本次验证码会话码
//  3. 识别 —— 多模态模型看图给答案
//  4. captcha/validate/scenesImage —— 用答案换 ticket
const (
	// 服务端按固定 boundary 解析 multipart（与 App 的 WebKit boundary 一致），
	// 用自定义 Content-Type 时保持一致最稳。
	captchaBoundary = "----WebKitFormBoundaryBlRdUZvbYBzP5FaF"
	// validate 失败后服务端要求等 15s 才能刷新验证码。
	captchaRetryWait = 16 * time.Second
	captchaMaxTry    = 2
)

// checkValidationURL 按任务类型返回验证码检查接口。
func checkValidationURL(host string, kind TaskKind) string {
	switch kind {
	case KindAttendance:
		return "https://" + host + "/wec-counselor-attendance-apps/student/attendance/checkValidation"
	case KindCollect:
		return "https://" + host + "/wec-counselor-collector-apps/stu/collector/checkValidation"
	default:
		return "https://" + host + "/wec-counselor-sign-apps/stu/sign/checkValidation"
	}
}

// CaptchaChallenge 是 checkValidation 返回的验证码会话参数。
type CaptchaChallenge struct {
	Required   bool
	AccountKey string
	SceneCode  string
	TenantID   string
	UserID     string
}

// CheckCaptchaValidation 询问服务端本次提交是否需要通过验证码。
// 请求失败不应阻断签到流程：对没开启验证码的学校，直接提交仍能成功。
func CheckCaptchaValidation(ctx context.Context, client *Client, host string, kind TaskKind, deviceID string) (*CaptchaChallenge, error) {
	body, _ := json.Marshal(map[string]string{"deviceId": deviceID})
	headers := map[string]string{
		"Content-Type":      "application/json; charset=utf-8",
		"CpdailyStandAlone": "0",
		"extension":         "1",
	}
	resp, err := client.do(ctx, http.MethodPost, checkValidationURL(host, kind), headers, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Datas struct {
			Validation bool   `json:"validation"`
			AccountKey string `json:"accountKey"`
			SceneCode  string `json:"sceneCode"`
			TenantID   string `json:"tenantId"`
			UserID     string `json:"userId"`
		} `json:"datas"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析验证码检查响应失败：%w", err)
	}
	ch := &CaptchaChallenge{Required: out.Datas.Validation}
	if ch.Required {
		ch.AccountKey = out.Datas.AccountKey
		ch.SceneCode = out.Datas.SceneCode
		ch.TenantID = out.Datas.TenantID
		ch.UserID = out.Datas.UserID
	}
	return ch, nil
}

// captchaMultipart 以固定 boundary 构造 multipart 表单。
func captchaMultipart(fields [][2]string) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	if err := w.SetBoundary(captchaBoundary); err != nil {
		return nil, err
	}
	for _, kv := range fields {
		if err := w.WriteField(kv[0], kv[1]); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

// captchaPost 发送 multipart 请求并解析 JSON 响应。
func (c *Client) captchaPost(ctx context.Context, url string, fields [][2]string) (map[string]json.RawMessage, error) {
	buf, err := captchaMultipart(fields)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Content-Type": "multipart/form-data; boundary=" + captchaBoundary,
		"deviceId":     c.cookies["deviceId"],
	}
	resp, err := c.do(ctx, http.MethodPost, url, headers, buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("验证码接口返回异常（HTTP %d）：%s", resp.StatusCode, truncateStr(string(raw), 160))
	}
	return out, nil
}

// captchaImage 从 create 响应的 result 里找出验证码图片。
// 响应结构没有公开文档：优先按常见键名找，找不到就取最长的
// base64 串（合法图片 base64 至少数百字节，其他字段都是短码）。
func captchaImage(result map[string]json.RawMessage) (mime string, b64 string, err error) {
	strip := func(s string) string {
		if i := strings.Index(s, ";base64,"); i >= 0 {
			return s[i+len(";base64,"):]
		}
		return s
	}
	for _, key := range []string{"image", "img", "file", "imageBase64", "base64", "data"} {
		v, ok := result[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) != nil || len(s) < 256 {
			continue
		}
		return "image/png", strip(s), nil
	}
	longest := ""
	for _, v := range result {
		var s string
		if json.Unmarshal(v, &s) != nil {
			continue
		}
		if len(s) > len(longest) {
			longest = s
		}
	}
	if len(longest) >= 512 {
		return "image/png", strip(longest), nil
	}
	return "", "", errors.New("验证码响应中未找到图片数据")
}

// CaptchaCreateForTest 仅诊断工具使用：调用 create 并返回原始响应，
// 用于观察真实响应结构（图片字段名没有公开文档）。不要在业务路径调用。
func CaptchaCreateForTest(ctx context.Context, client *Client, host string, ch *CaptchaChallenge) (map[string]json.RawMessage, error) {
	base := "https://" + host + "/captcha-open-api/v1/captcha/"
	fields := [][2]string{
		{"accountKey", ch.AccountKey},
		{"sceneCode", ch.SceneCode},
		{"tenantId", ch.TenantID},
		{"userId", ch.UserID},
	}
	return client.captchaPost(ctx, base+"create/scenesImage", fields)
}

// solveCaptcha 走完整验证码链路，返回可随提交体携带的 ticket。
func (c *Client) solveCaptcha(ctx context.Context, host string, ch *CaptchaChallenge) (string, error) {
	base := "https://" + host + "/captcha-open-api/v1/captcha/"
	baseFields := [][2]string{
		{"accountKey", ch.AccountKey},
		{"sceneCode", ch.SceneCode},
		{"tenantId", ch.TenantID},
		{"userId", ch.UserID},
	}
	var lastErr error
	for try := 0; try < captchaMaxTry; try++ {
		if try > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(captchaRetryWait):
			}
		}
		// ① create：拿验证码图与会话码
		createOut, err := c.captchaPost(ctx, base+"create/scenesImage", baseFields)
		if err != nil {
			lastErr = err
			continue
		}
		var result map[string]json.RawMessage
		if raw := createOut["result"]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &result)
		}
		code := jsonString(result, "code")
		if code == "" {
			lastErr = errors.New("验证码接口未返回会话码：" + flattenJSON(createOut))
			continue
		}
		// ② 识别
		mime, imgB64, err := captchaImage(result)
		if err != nil {
			lastErr = err
			continue
		}
		answer, err := vision.DescribeCaptcha(ctx, mime, imgB64)
		if err != nil {
			lastErr = err
			continue
		}
		answers := parseAnswers(answer)
		if len(answers) == 0 {
			lastErr = fmt.Errorf("识别结果无法解析：%s", truncateStr(answer, 120))
			continue
		}
		// ③ validate：答案换 ticket
		fields := append(append([][2]string{}, baseFields...),
			[2]string{"scenesImageCode", code})
		for _, a := range answers {
			fields = append(fields, [2]string{"scenesImageCodes", a})
		}
		validateOut, err := c.captchaPost(ctx, base+"validate/scenesImage", fields)
		if err != nil {
			lastErr = err
			continue
		}
		ticket := jsonString(validateOut, "result")
		if ticket == "" {
			// 答案错误或已超时：等 15s 刷新新码再来一轮
			lastErr = errors.New("验证码校验未通过：" + flattenJSON(validateOut))
			continue
		}
		return ticket, nil
	}
	return "", lastErr
}

// captchaTicketFor 是 submit 的入口：需要验证码时返回 ticket，否则返回空串。
// checkValidation 自身的网络错误不阻断提交（未开启验证码的学校不需要它）。
func (c *Client) captchaTicketFor(ctx context.Context, host string, kind TaskKind, deviceID string) (string, error) {
	ch, err := CheckCaptchaValidation(ctx, c, host, kind, deviceID)
	if err != nil {
		return "", nil
	}
	if !ch.Required {
		return "", nil
	}
	if !vision.Enabled() {
		return "", errors.New("该校签到需要通过图形验证码，但服务端未配置识别服务（DEEPSEEK_API_KEY）")
	}
	return c.solveCaptcha(ctx, host, ch)
}

// parseAnswers 从模型回答里提取答案列表。
// 模型可能输出 JSON 数组、带括号的列表或单个词，尽量宽容。
func parseAnswers(answer string) []string {
	answer = strings.TrimSpace(answer)
	// 去掉 ```json 围栏
	answer = strings.TrimPrefix(answer, "```json")
	answer = strings.TrimPrefix(answer, "```")
	answer = strings.TrimSuffix(answer, "```")
	answer = strings.TrimSpace(answer)
	var arr []string
	if err := json.Unmarshal([]byte(answer), &arr); err == nil {
		out := make([]string, 0, len(arr))
		for _, a := range arr {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, a)
			}
		}
		return out
	}
	// 逗号/顿号分隔
	for _, sep := range []string{"，", ",", "、"} {
		answer = strings.ReplaceAll(answer, sep, "|")
	}
	parts := strings.Split(answer, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"'“”[]()（）`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// jsonString 从 map 里取字符串字段。
func jsonString(m map[string]json.RawMessage, key string) string {
	if m == nil {
		return ""
	}
	var s string
	if raw, ok := m[key]; ok && json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// flattenJSON 把 JSON map 压成一行短串，用于错误信息。
func flattenJSON(m map[string]json.RawMessage) string {
	raw, _ := json.Marshal(m)
	return truncateStr(string(raw), 200)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
