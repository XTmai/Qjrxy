package cpdaily

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/vision"
)

// 「以图判图」验证码：部分学校的签到/查寝在提交前要求通过一个
// 场景验证码，通过后签发一张一次性 ticket，随提交体一起走 AES + sign。
// 缺 ticket 的提交会被服务端以「验证码验签失败」(2210060000) 拒绝。
//
// 它实际上是**九图选择题**，不是问答题：create 下发 9 张图和一个目标
// 词（如「自行车」），登录方要挑出含该物品的格子，把这些格子各自的
// code 填进 scenesImageCodes。详见参考实现 ruoli-sign-optimization
// 的 documents/开发者文档.md。
//
// 完整链路：
//  1. checkValidation —— 确认确实要验证码，拿到 accountKey 等参数
//  2. captcha/create/scenesImage —— 拿 9 张候选图（{code,path}）、目标词、会话码
//  3. 识别 —— 9 张图拼成九宫格，一次多模态请求问「哪几格有目标物」
//  4. captcha/validate/scenesImage —— 用命中的图 code 换 ticket
//
// 调用时机：只有提交返回 2210060000 之后才会走这里。不需要验证码的
// 学校，请求序列与引入该功能前完全一致。
const (
	// 服务端按固定 boundary 解析 multipart（与 App 的 WebKit boundary 一致），
	// 用自定义 Content-Type 时保持一致最稳。
	captchaBoundary = "----WebKitFormBoundaryBlRdUZvbYBzP5FaF"
	captchaMaxTry   = 3
	// captchaRequiredCode 是服务端「验证码验签失败」的业务码。
	captchaRequiredCode = "2210060000"
	// 九宫格单格边长与并发下载数。720×720 的 JPEG 约 100KB，
	// 相比 9 张原图分别上传，token 与请求数都省一个量级。
	captchaGridCell   = 240
	captchaFetchLimit = 4
	captchaImageMax   = 400 << 10
)

// captchaRetryWait 是服务端要求的最小刷新间隔（15s，留 1s 余量）。
// 声明为变量只为让测试把它压到毫秒级，业务代码不应改写。
var captchaRetryWait = 16 * time.Second

// captchaProbeTimeout 是提交前 checkValidation 探路的超时。
//
// 它只用于「这次要不要验证码」这一个判断：正常响应是几十毫秒的小 JSON。
// 给太短会误判成"不需要"（随后提交白失败一次），给太长又会让抽风的学校
// 拖住整个签到——参考实现里没有超时，实测会白等 40 秒。5 秒是折中。
// 声明为变量只为让测试把它压到毫秒级，业务代码不应改写。
var captchaProbeTimeout = 5 * time.Second

// captchaRecognizeRetryWait 是「识别失败、换一张重来」的间隔。
//
// 与 validate 失败不同：识别失败时服务端还没开始计时，只需要让用户/模型
// 换一张图。参考实现这里只等 5-6 秒，而 validate 失败要等 16-20 秒
// （服务端限制）。分成两种间隔能少等三分之一。
var captchaRecognizeRetryWait = 5 * time.Second

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

// ManualCaptchaRequest 是交给用户人工判定的验证码题目。
type ManualCaptchaRequest struct {
	// Name 是要找的目标物，如「自行车」。
	Name string
	// Grid 是 3×3 九宫格的 data URL。直接给拼好的图而不是 9 个外链：
	// 图片服务器要求带 Cookie 与 Referer，浏览器直连大概率 403。
	Grid string
	// Cells 是格子总数（恒为 9，按接口下发值给）。
	Cells int
}

// ManualCaptchaFunc 在自动识别不可用时把题目交给用户。
// 返回 1 基的格子序号；返回 ErrManualCaptchaAborted 表示用户放弃，
// 调用方应立即结束整条验证码链路，而不是重试。
type ManualCaptchaFunc func(ctx context.Context, req ManualCaptchaRequest) ([]int, error)

// ErrManualCaptchaAborted 表示用户主动取消或等待超时。
var ErrManualCaptchaAborted = errors.New("已放弃手动验证")

// manualAbortError 带一句给人看的原因，同时能被 errors.Is 认成
// ErrManualCaptchaAborted —— 上层据此决定「不要再弹第二次」，
// 界面则能显示「你取消了」还是「等太久了」。
type manualAbortError struct{ reason string }

func (e *manualAbortError) Error() string { return e.reason }

func (e *manualAbortError) Is(target error) bool { return target == ErrManualCaptchaAborted }

// AbortManualCaptcha 构造一条带原因的放弃信号，交给上层作为最终提示。
func AbortManualCaptcha(reason string) error {
	return &manualAbortError{reason: reason}
}

// ErrManualCaptchaSkip 表示用户要求换一张（当前这张看不清 / 没有目标物）。
var ErrManualCaptchaSkip = errors.New("请求更换验证码")

// CaptchaChallenge 是 checkValidation 返回的验证码会话参数。
type CaptchaChallenge struct {
	Required   bool
	AccountKey string
	SceneCode  string
	TenantID   string
	UserID     string
}

// CheckCaptchaValidation 询问服务端本次提交是否需要通过验证码。
//
// 只在提交已被 2210060000 拒绝后调用，因此这里的错误是硬错误，
// 由调用方翻译成「拿不到验证码会话」，而不是静默跳过。
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
		return nil, fmt.Errorf("解析验证码检查响应失败（HTTP %d）：%s", resp.StatusCode, truncateStr(string(raw), 160))
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
//
// deviceId 必须走请求头：服务端用它把验证码会话与设备绑定，缺失时
// create 会被拒。它不在 cookie 里，只能由调用方显式传入。
func (c *Client) captchaPost(ctx context.Context, url, deviceID string, fields [][2]string) (map[string]json.RawMessage, error) {
	buf, err := captchaMultipart(fields)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Content-Type": "multipart/form-data; boundary=" + captchaBoundary,
		"deviceId":     deviceID,
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

// captchaImageInfo 是 create 下发的一张候选图。code 是提交答案时
// 唯一被认可的值，path 是图片地址。
type captchaImageInfo struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// captchaCreateResult 是 create 的 result 体。
type captchaCreateResult struct {
	Code       string             `json:"code"` // 本次会话码，validate 时作为 scenesImageCode
	Name       string             `json:"name"` // 要找的目标物
	ImageInfos []captchaImageInfo `json:"imageInfos"`
	// 兼容老结构：少数部署仍返回单图 base64。
	Image string `json:"image"`
}

// parseCaptchaCreate 从 create 响应里取 result。
func parseCaptchaCreate(out map[string]json.RawMessage) (*captchaCreateResult, error) {
	raw := out["result"]
	if len(raw) == 0 {
		return nil, errors.New("验证码接口未返回 result：" + flattenJSON(out))
	}
	var cr captchaCreateResult
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, fmt.Errorf("解析验证码响应失败：%w（原文 %s）", err, truncateStr(string(raw), 160))
	}
	return &cr, nil
}

// fetchCaptchaImage 下载一张候选图。
//
// 图片在独立的 CDN 域上（如 wecloud-captcha.campusphere.net），只需要
// 图片本身，不需要业务会话——因此不发送 Cookie，避免把 MOD_AUTH_CAS
// 交给第三方主机。部分部署按 Referer 做防盗链，带上图片自身的来源域。
func (c *Client) fetchCaptchaImage(ctx context.Context, rawURL string) (image.Image, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return nil, fmt.Errorf("验证码图片地址不合法：%s", truncateStr(rawURL, 80))
	}
	headers := map[string]string{
		"Accept":  "image/*",
		"Referer": refererOf(rawURL),
	}
	resp, err := c.doNoCookie(ctx, http.MethodGet, rawURL, headers, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载验证码图片失败（HTTP %d）", resp.StatusCode)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, captchaImageMax))
	if err != nil {
		return nil, fmt.Errorf("解码验证码图片失败：%w", err)
	}
	return img, nil
}

// refererOf 用图片地址的协议 + 主机作为 Referer，只保留来源域，
// 不把具体图片路径再带回去。
func refererOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// fetchCaptchaImages 并发下载 9 张候选图。下载失败的格子留空（白底），
// 拼图仍可继续——模型看到空白格大概率判定为「不包含」。
func (c *Client) fetchCaptchaImages(ctx context.Context, paths []string) []image.Image {
	out := make([]image.Image, len(paths))
	sem := make(chan struct{}, captchaFetchLimit)
	var wg sync.WaitGroup
	for i, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if img, err := c.fetchCaptchaImage(ctx, p); err == nil {
				out[i] = img
			}
		}(i, p)
	}
	wg.Wait()
	return out
}

// composeGrid 把若干张图按 3×3 拼成一张，格子间画浅灰分割线，
// 便于模型分辨边界。顺序固定为从左到右、从上到下。
func composeGrid(imgs []image.Image, cell int) *image.RGBA {
	const n = 3
	side := cell * n
	canvas := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	for i := 0; i < n*n && i < len(imgs); i++ {
		img := imgs[i]
		if img == nil {
			continue
		}
		b := img.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			continue
		}
		// 留 6px 内边距，等比缩放后居中
		box := cell - 12
		scale := math.Min(float64(box)/float64(b.Dx()), float64(box)/float64(b.Dy()))
		w := int(float64(b.Dx()) * scale)
		h := int(float64(b.Dy()) * scale)
		if w <= 0 || h <= 0 {
			continue
		}
		ox, oy := (i%n)*cell, (i/n)*cell
		dst := image.Rect(ox+(cell-w)/2, oy+(cell-h)/2, ox+(cell-w)/2+w, oy+(cell-h)/2+h)
		resizeBilinear(canvas, dst, img)
	}

	// 分割线：2px 浅灰，帮助模型定位格子
	line := color.RGBA{R: 210, G: 214, B: 220, A: 255}
	for k := 1; k < n; k++ {
		for t := 0; t < side; t++ {
			for d := 0; d < 2; d++ {
				canvas.Set(k*cell+d-1, t, line)
				canvas.Set(t, k*cell+d-1, line)
			}
		}
	}
	return canvas
}

// resizeBilinear 把 src 缩放到 dst（双线性插值）。标准库没有缩放，
// 引 x/image 只为一个函数不划算，这里手写一份够用。
func resizeBilinear(dst *image.RGBA, rect image.Rectangle, src image.Image) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := rect.Dx(), rect.Dy()
	if sw == 0 || sh == 0 || dw == 0 || dh == 0 {
		return
	}
	for y := 0; y < dh; y++ {
		// 对齐像素中心，避免整体偏移半格
		sy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
		}
		for x := 0; x < dw; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}
			r00, g00, b00, a00 := src.At(sb.Min.X+x0, sb.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := src.At(sb.Min.X+x1, sb.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := src.At(sb.Min.X+x0, sb.Min.Y+y1).RGBA()
			r11, g11, b11, a11 := src.At(sb.Min.X+x1, sb.Min.Y+y1).RGBA()
			lerp := func(v00, v10, v01, v11 uint32) uint8 {
				top := float64(v00)*(1-fx) + float64(v10)*fx
				bot := float64(v01)*(1-fx) + float64(v11)*fx
				return uint8((top*(1-fy) + bot*fy) / 257)
			}
			dst.Set(rect.Min.X+x, rect.Min.Y+y, color.RGBA{
				R: lerp(r00, r10, r01, r11),
				G: lerp(g00, g10, g01, g11),
				B: lerp(b00, b10, b01, b11),
				A: lerp(a00, a10, a01, a11),
			})
		}
	}
}

// encodeJPEG 把拼图编码成 JPEG。JPEG 比 PNG 小一个量级，720×720
// 在 q85 下约 100KB，模型侧延迟也更低。
func encodeJPEG(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// solveCaptcha 走完整验证码链路，返回可随提交体携带的 ticket。
//
// manual 非空时作为兜底：模型没配置、调用失败或没找到目标物，就把
// 同一张九宫格交给用户点选。走到人工这一步说明自动识别已经尽力，
// 不再重复调模型——省一次必然失败的请求，也让用户少等一轮。
func (c *Client) solveCaptcha(ctx context.Context, host string, ch *CaptchaChallenge, deviceID string, manual ManualCaptchaFunc) (string, bool, error) {
	base := "https://" + host + "/captcha-open-api/v1/captcha/"
	baseFields := [][2]string{
		{"accountKey", ch.AccountKey},
		{"sceneCode", ch.SceneCode},
		{"tenantId", ch.TenantID},
		{"userId", ch.UserID},
	}
	var lastErr error
	usedManual := false
	// validate 失败才需要等满服务端的 15 秒刷新限制；识别失败时服务端
	// 还没开始计时，换一张只要几秒。区分开来能少等三分之一。
	validateRejected := false
	for try := 0; try < captchaMaxTry; try++ {
		if try > 0 {
			wait := captchaRecognizeRetryWait
			if validateRejected {
				wait = captchaRetryWait
			}
			// 按尝试次数小幅错开，避免固定的整点间隔显得像脚本。
			wait += time.Duration(try*1500) * time.Millisecond
			select {
			case <-ctx.Done():
				return "", usedManual, ctx.Err()
			case <-time.After(wait):
			}
		}
		validateRejected = false
		// ① create：拿 9 张候选图、目标词与会话码
		createOut, err := c.captchaPost(ctx, base+"create/scenesImage", deviceID, baseFields)
		if err != nil {
			lastErr = err
			continue
		}
		cr, err := parseCaptchaCreate(createOut)
		if err != nil {
			lastErr = err
			continue
		}
		code := strings.TrimSpace(cr.Code)
		if code == "" {
			lastErr = errors.New("验证码接口未返回会话码：" + flattenJSON(createOut))
			continue
		}
		// ② 取图：拼成九宫格，自动与人工共用同一张
		grid, total, err := c.buildCaptchaGrid(ctx, cr)
		if err != nil {
			lastErr = err
			continue
		}
		// ③ 识别：先让模型挑，挑不出来再交给用户
		cells, vErr := c.pickByVision(ctx, grid, cr.Name, total)
		if vErr != nil || len(cells) == 0 {
			if manual == nil {
				if vErr != nil {
					lastErr = vErr
				} else {
					lastErr = errors.New("自动识别未在九张图中找到目标物")
				}
				continue
			}
			usedManual = true
			cells, err = manual(ctx, ManualCaptchaRequest{Name: cr.Name, Grid: grid, Cells: total})
			if err != nil {
				// 用户放弃就整条链路结束：再弹一次只会更烦。
				if errors.Is(err, ErrManualCaptchaAborted) {
					return "", true, err
				}
				lastErr = err
				continue
			}
		}
		codes := codesForCells(cr, cells)
		if len(codes) == 0 {
			lastErr = errors.New("没有选出任何包含目标物的图片")
			continue
		}
		// ④ validate：命中图的 code 换 ticket
		fields := append(append([][2]string{}, baseFields...),
			[2]string{"scenesImageCode", code})
		for _, cd := range codes {
			fields = append(fields, [2]string{"scenesImageCodes", cd})
		}
		validateOut, err := c.captchaPost(ctx, base+"validate/scenesImage", deviceID, fields)
		if err != nil {
			lastErr = err
			validateRejected = true
			continue
		}
		ticket := jsonString(validateOut, "result")
		if ticket == "" {
			lastErr = errors.New("验证码校验未通过：" + flattenJSON(validateOut))
			validateRejected = true
			continue
		}
		return ticket, usedManual, nil
	}
	if lastErr == nil {
		lastErr = errors.New("验证码处理失败")
	}
	return "", usedManual, lastErr
}

// buildCaptchaGrid 下载 9 张候选图并拼成九宫格，返回 data URL 与可用格子数。
//
// 自动识别与人工判定用的是同一张图：拼一次、两边复用，人工路径下
// 用户在浏览器里看到的就是模型看到的那张，避免两条路径对不上。
func (c *Client) buildCaptchaGrid(ctx context.Context, cr *captchaCreateResult) (string, int, error) {
	if len(cr.ImageInfos) == 0 {
		return "", 0, errors.New("验证码响应里没有候选图（imageInfos 为空）")
	}
	paths := make([]string, len(cr.ImageInfos))
	for i, info := range cr.ImageInfos {
		paths[i] = info.Path
	}
	imgs := c.fetchCaptchaImages(ctx, paths)
	downloaded := 0
	for _, img := range imgs {
		if img != nil {
			downloaded++
		}
	}
	if downloaded == 0 {
		return "", 0, errors.New("验证码图片全部下载失败，请检查网络或学校图片服务器")
	}

	grid := composeGrid(imgs, captchaGridCell)
	gridB64, err := encodeJPEG(grid)
	if err != nil {
		return "", 0, fmt.Errorf("编码九宫格失败：%w", err)
	}
	return "data:image/jpeg;base64," + gridB64, len(cr.ImageInfos), nil
}

// pickByVision 让多模态模型挑出命中目标物的格子。
func (c *Client) pickByVision(ctx context.Context, gridB64, name string, total int) ([]int, error) {
	const prefix = "data:image/jpeg;base64,"
	b64 := strings.TrimPrefix(gridB64, prefix)
	return vision.PickCaptchaCells(ctx, "image/jpeg", b64, name, total)
}

// codesForCells 把 1 基格子序号映射回各自的 code。
// 越界序号与空 code 一律丢弃——宁可直接判失败，也不要拿空值去 validate。
func codesForCells(cr *captchaCreateResult, cells []int) []string {
	codes := make([]string, 0, len(cells))
	seen := make(map[string]bool, len(cells))
	for _, cell := range cells {
		if cell < 1 || cell > len(cr.ImageInfos) {
			continue
		}
		cd := strings.TrimSpace(cr.ImageInfos[cell-1].Code)
		if cd == "" || seen[cd] {
			continue
		}
		seen[cd] = true
		codes = append(codes, cd)
	}
	return codes
}

// errCaptchaNotRequired 表示提交被 2210060000 拒绝、但 checkValidation
// 又说不需要验证码（学校侧状态不一致）。这种情况应把原始失败结果
// 交回调用方，而不是包装成「验证码处理失败」。
var errCaptchaNotRequired = errors.New("checkValidation 显示不需要验证码")

// captchaTicketFor 在提交被 2210060000 拒绝后调用：确认要验证码并换取 ticket。
// 走到这里说明提交确实需要 ticket，任何一步失败都应报错，不再静默跳过。
func (c *Client) captchaTicketFor(ctx context.Context, host string, kind TaskKind, deviceID string, manual ManualCaptchaFunc) (string, bool, error) {
	ch, err := CheckCaptchaValidation(ctx, c, host, kind, deviceID)
	if err != nil {
		return "", false, fmt.Errorf("获取验证码会话失败：%w", err)
	}
	if !ch.Required {
		return "", false, errCaptchaNotRequired
	}
	// 自动识别与人工兜底都没有，才是真的无解。走到这里的是定时任务：
	// 那个时间点没人看屏幕，只能提示用户去手动执行一次。
	if !vision.Enabled() && manual == nil {
		return "", false, errors.New("该校签到需要图形验证码：定时任务无人值守，请到任务页手动执行一次，在弹窗里选出目标图片")
	}
	return c.solveCaptcha(ctx, host, ch, deviceID, manual)
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
