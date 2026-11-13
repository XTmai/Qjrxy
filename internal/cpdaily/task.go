package cpdaily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DeviceProfile 是提交请求里声明的设备信息，必须与账号绑定的设备一致才会通过设备校验。
type DeviceProfile struct {
	DeviceID      string `json:"deviceId"`
	DeviceModel   string `json:"deviceModel"`
	SystemVersion string `json:"systemVersion"`
	BuildID       string `json:"buildId"`
	SystemName    string `json:"systemName"`
	AppVersion    string `json:"appVersion"`
	// Platform 为空或 "android" 走 Android 协议；"ios" 走 iOS 协议。
	Platform string `json:"platform"`
	// WisDeviceID 是 iOS 端的设备指纹请求头，仅 iOS 协议发送。
	WisDeviceID string `json:"wisDeviceId"`
	// RawUserAgent 是扫码时 App WebView 的真实 UA。
	// 非空时原样使用，保证请求特征与真机一致（优先级高于拼接）。
	RawUserAgent string  `json:"userAgent"`
	UserID       string  `json:"-"`
	Lon          float64 `json:"lon"`
	Lat          float64 `json:"lat"`
}

// IsIOS 判断是否使用 iOS 协议。
func (d DeviceProfile) IsIOS() bool {
	switch strings.ToLower(strings.TrimSpace(d.Platform)) {
	case "ios", "iphone", "ipad", "macos":
		return true
	}
	return false
}

// UserAgent 依据设备信息生成 App 的 UA。
func (d DeviceProfile) UserAgent() string {
	// 扫码拿到的原始 UA 优先：它直接来自今日校园 App 的 WebView，
	// 与真机请求完全一致，比自己拼接更不易被识别。
	if ua := strings.TrimSpace(d.RawUserAgent); ua != "" {
		return ua
	}
	model := d.DeviceModel
	if model == "" {
		model = "23127PN0CC"
	}
	version := d.SystemVersion
	if version == "" {
		version = "14"
	}
	build := d.BuildID
	if build == "" {
		build = DefaultBuildID
	}
	app := d.AppVersion
	if app == "" {
		app = AppVersion
	}
	if d.IsIOS() {
		// iOS UA：OS 版本用下划线，无 Build 段，无 okhttp
		return fmt.Sprintf("Mozilla/5.0 (iPhone; CPU iPhone OS %s like Mac OS X) "+
			"AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 "+
			"cpdaily/%s wisedu/%s", version, app, app)
	}
	return fmt.Sprintf("Mozilla/5.0 (Linux; Android %s; %s Build/%s; wv) AppleWebKit/537.36 "+
		"(KHTML, like Gecko) Version/4.0 Chrome/120.0.6099.230 Mobile Safari/537.36 "+
		"okhttp/4.12.0 cpdaily/%s wisedu/%s", version, model, build, app, app)
}

// Task 是任务列表里的一条待办。
type Task struct {
	Kind            TaskKind `json:"kind"`
	KindLabel       string   `json:"kindLabel"`
	SignInstanceWID string   `json:"signInstanceWid"`
	SignWID         string   `json:"signWid"`
	StuSignWID      string   `json:"stuSignWid"`
	TaskName        string   `json:"taskName"`
	Sender          string   `json:"sender"`
	// StartTime/EndTime 用于展示：优先用单次任务时间，其次用周期性时段
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Status    string `json:"status"`

	// 周期性任务的时间信息
	SignDate   string `json:"signDate"`   // 如 "2026-11-03 (周二)"
	BeginClock string `json:"beginClock"` // 如 "14:00"
	EndClock   string `json:"endClock"`   // 如 "14:20"
	// SignStatus：0/1 未签到，2 未到时间，3 已过期，4 已签到
	SignStatus     string `json:"signStatus"`
	SignStatusText string `json:"signStatusText"`
	TaskType       string `json:"taskType"`
}

// TaskDetail 是任务详情（仅取提交所需字段）。
type TaskDetail struct {
	SignInstanceWID string       `json:"signInstanceWid"`
	TaskName        string       `json:"taskName"`
	IsPhoto         FlexInt      `json:"isPhoto"`
	IsNeedExtra     FlexInt      `json:"isNeedExtra"`
	IsMalposition   FlexInt      `json:"isMalposition"`
	ExtraFields     []ExtraField `json:"extraField"`
	SignPlaces      []SignPlace  `json:"signPlaceSelected"`
	// SignStatus 是详情里的签到状态（0/1 未签，2 未到时间，3 已过期，4 已签到）。
	// 提交前用它挡掉「已经签过」的任务，避免重试路径重复提交。
	SignStatus FlexString      `json:"signStatus"`
	SignTime   FlexString      `json:"signTime"`
	Raw        json.RawMessage `json:"-"`
}

// FlexString 兼容服务端把字符串写成数字的情况（如 signStatus: 4）。
type FlexString string

// UnmarshalJSON 接受字符串或数字，统一存成去掉引号的字符串。
func (s *FlexString) UnmarshalJSON(b []byte) error {
	str := strings.TrimSpace(string(b))
	if str == "null" {
		*s = ""
		return nil
	}
	str = strings.Trim(str, `"`)
	*s = FlexString(str)
	return nil
}

// ExtraField 是签到附加题。
type ExtraField struct {
	Title string `json:"title"`
	Items []struct {
		WID          string  `json:"wid"`
		Content      string  `json:"content"`
		IsSelected   FlexInt `json:"isSelected"`
		IsOtherItems FlexInt `json:"isOtherItems"`
	} `json:"extraFieldItems"`
}

// SignPlace 是签到点。
type SignPlace struct {
	PlaceName string    `json:"placeName"`
	Address   string    `json:"address"`
	Longitude FlexFloat `json:"longitude"`
	Latitude  FlexFloat `json:"latitude"`
	Radius    FlexFloat `json:"radius"`
}

// FlexInt 兼容服务端把整数写成字符串或数字的情况。
type FlexInt int

func (i *FlexInt) UnmarshalJSON(b []byte) error {
	str := strings.TrimSpace(string(b))
	if str == "" || str == "null" {
		*i = 0
		return nil
	}
	str = strings.Trim(str, `"`)
	if str == "" {
		*i = 0
		return nil
	}
	// 先按整数解析，失败再按浮点截断
	if v, err := strconv.Atoi(str); err == nil {
		*i = FlexInt(v)
		return nil
	}
	if v, err := strconv.ParseFloat(str, 64); err == nil {
		*i = FlexInt(int(v))
		return nil
	}
	*i = 0
	return nil
}

// Int 返回 int 值。
func (i FlexInt) Int() int { return int(i) }

// FlexFloat 兼容服务端把数字写成字符串或数字的情况。
//
// 实测（iOS 抓包）：signPlaceSelected 里 longitude/latitude 是字符串
// "111.2195637741102"，而 radius 是数字 1500。用 float64 直接解析会报
// "cannot unmarshal string into Go struct field ... of type float64"。
type FlexFloat float64

func (f *FlexFloat) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	// 去掉可能存在的引号
	s = strings.Trim(s, `"`)
	if s == "" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// 无法解析时归零，不让整条任务详情解析失败
		*f = 0
		return nil
	}
	*f = FlexFloat(v)
	return nil
}

// Float 返回 float64 值。
func (f FlexFloat) Float() float64 { return float64(f) }

// rawTask 是列表接口返回的单条任务。
type rawTask struct {
	SignInstanceWID string `json:"signInstanceWid"`
	SignWID         string `json:"signWid"`
	StuSignWID      string `json:"stuSignWid"`
	TaskName        string `json:"taskName"`
	Sender          string `json:"senderUserName"`
	SignStatus      string `json:"signStatus"`
	TaskType        string `json:"taskType"`
	RateSignDate    string `json:"rateSignDate"`
	RateBeginTime   string `json:"rateTaskBeginTime"`
	RateEndTime     string `json:"rateTaskEndTime"`
	SingleBegin     string `json:"singleTaskBeginTime"`
	SingleEnd       string `json:"singleTaskEndTime"`
}

// taskLists 是当日任务列表的三个分区。
type taskLists struct {
	UnSigned []rawTask
	Signed   []rawTask
	Leave    []rawTask
}

// fetchTaskLists 拉取某类任务的当日列表（含已签到分区）。
//
// 第一次请求用于获取 cookie（部分学校需要），忽略结果。
func (c *Client) fetchTaskLists(ctx context.Context, host string, kind TaskKind) (*taskLists, error) {
	// host 可能是用户手填的（带 scheme/路径），拼上 "https://" 就会变成
	// https://https://…；这里统一成纯域名，所有任务请求都经过这一处。
	host = NormalizeHost(host)
	if host == "" {
		return nil, errors.New("学校域名无效，请检查账号的 host 设置")
	}
	endpoints := EndpointsFor(kind)
	if _, err := c.postJSON(ctx, "https://"+host+endpoints.List, nil, nil); err != nil {
		return nil, err
	}
	resp, err := c.postJSON(ctx, "https://"+host+endpoints.List, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		UnSigned []rawTask `json:"unSignedTasks"`
		Signed   []rawTask `json:"signedTasks"`
		Leave    []rawTask `json:"leaveTasks"`
	}
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return nil, fmt.Errorf("解析任务列表失败：%w", err)
	}
	return &taskLists{UnSigned: payload.UnSigned, Signed: payload.Signed, Leave: payload.Leave}, nil
}

// buildTask 把列表里的一条原始任务转成 Task。
func buildTask(t rawTask, kind TaskKind, status string) Task {
	start, end := t.SingleBegin, t.SingleEnd
	// 单次任务时间为空时，用「日期 + 时段」组装展示文本
	if start == "" && t.RateSignDate != "" {
		date := strings.SplitN(t.RateSignDate, " ", 2)[0]
		if t.RateBeginTime != "" {
			start = date + " " + t.RateBeginTime
		} else {
			start = date
		}
		if t.RateEndTime != "" {
			end = date + " " + t.RateEndTime
		}
	}
	return Task{
		Kind: kind, KindLabel: kind.Label(),
		SignInstanceWID: t.SignInstanceWID,
		SignWID:         t.SignWID,
		StuSignWID:      t.StuSignWID,
		TaskName:        t.TaskName,
		Sender:          t.Sender,
		StartTime:       start,
		EndTime:         end,
		Status:          status,
		SignDate:        t.RateSignDate,
		BeginClock:      t.RateBeginTime,
		EndClock:        t.RateEndTime,
		SignStatus:      t.SignStatus,
		SignStatusText:  signStatusText(t.SignStatus),
		TaskType:        t.TaskType,
	}
}

// listTasks 查询某类任务的未完成列表（待办 + 请假；已签到不进待办缓存）。
func (c *Client) listTasks(ctx context.Context, host string, kind TaskKind) ([]Task, error) {
	lists, err := c.fetchTaskLists(ctx, host, kind)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(lists.UnSigned)+len(lists.Leave))
	for _, t := range lists.UnSigned {
		out = append(out, buildTask(t, kind, "pending"))
	}
	for _, t := range lists.Leave {
		out = append(out, buildTask(t, kind, "leave"))
	}
	return out, nil
}

// TaskSignState 是单个任务在当日列表里的状态。
type TaskSignState int

const (
	// TaskUnsigned 任务仍在待办（未签到，或学校列表尚未更新）。
	TaskUnsigned TaskSignState = iota
	// TaskSigned 学校已把任务挪进已签到列表（正判）。
	TaskSigned
	// TaskMissing 两个列表里都找不到（可能已被学校撤销）。
	TaskMissing
)

// FetchTaskSignState 查询指定任务当天的签到状态。
//
// 用「signedTasks 里有它」做正判，替代「从待办里消失」的反向推断：
// 后者区分不了已签到与被撤销，也读不出列表延迟——实测部分学校
// （siit、nut）提交成功后约 20 秒内列表仍是旧数据。
func (c *Client) FetchTaskSignState(ctx context.Context, host string, kind TaskKind, signInstanceWid string) (TaskSignState, error) {
	lists, err := c.fetchTaskLists(ctx, host, kind)
	if err != nil {
		return TaskMissing, err
	}
	for _, t := range lists.Signed {
		if t.SignInstanceWID == signInstanceWid {
			return TaskSigned, nil
		}
	}
	for _, t := range lists.UnSigned {
		if t.SignInstanceWID == signInstanceWid {
			return TaskUnsigned, nil
		}
	}
	return TaskMissing, nil
}

// FetchSignedTasks 返回当日已签到的任务（用于事后回填「待核对」记录）。
func (c *Client) FetchSignedTasks(ctx context.Context, host string, kind TaskKind) ([]Task, error) {
	lists, err := c.fetchTaskLists(ctx, host, kind)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(lists.Signed))
	for _, t := range lists.Signed {
		out = append(out, buildTask(t, kind, "done"))
	}
	return out, nil
}

// MultiError 汇总多个任务的查询错误，支持 errors.As 下钻。
type MultiError struct {
	Errors []error
}

func (m *MultiError) Error() string {
	parts := make([]string, 0, len(m.Errors))
	for _, err := range m.Errors {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "；")
}

// Unwrap 返回所有子错误。
func (m *MultiError) Unwrap() []error { return m.Errors }

// ListAllTasks 查询多类任务的未完成列表，单类失败不影响其他类；
// 全部失败时返回 MultiError，部分失败时返回已取到的任务与错误。
func (c *Client) ListAllTasks(ctx context.Context, host string, kinds []TaskKind) ([]Task, error) {
	var tasks []Task
	var errs []error
	for _, kind := range kinds {
		list, err := c.listTasks(ctx, host, kind)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s：%w", kind.Label(), err))
			continue
		}
		tasks = append(tasks, list...)
	}
	if len(errs) == 0 {
		return tasks, nil
	}
	multi := &MultiError{Errors: errs}
	if len(tasks) == 0 {
		return nil, multi
	}
	return tasks, multi
}

// signStatusText 把 signStatus 转成可读文案。
//
// 实测取值：2 = 还没到签到时间（最常见），4 = 已签到，3 = 已过期。
func signStatusText(v string) string {
	switch strings.TrimSpace(v) {
	case "0", "1":
		return "可签到"
	case "2":
		return "未到时间"
	case "3":
		return "已过期"
	case "4":
		return "已签到"
	}
	return ""
}

// FetchDetail 拉取任务详情。
func (c *Client) FetchDetail(ctx context.Context, host string, kind TaskKind, task Task) (*TaskDetail, error) {
	endpoints := EndpointsFor(kind)
	// 签到与查寝的详情接口都要求 signInstanceWid + signWid 配对。
	// 实测（iOS 抓包）：查寝漏传 signWid 会导致后续提交报
	// 「查寝任务wid不能为空（code=22100100000）」。
	params := map[string]any{"signInstanceWid": task.SignInstanceWID, "signWid": task.SignWID}
	resp, err := c.postJSON(ctx, "https://"+host+endpoints.Detail, nil, params)
	if err != nil {
		return nil, err
	}
	detail := &TaskDetail{Raw: resp.Data}
	if err := json.Unmarshal(resp.Data, detail); err != nil {
		return nil, fmt.Errorf("解析任务详情失败：%w", err)
	}
	if detail.SignInstanceWID == "" {
		detail.SignInstanceWID = task.SignInstanceWID
	}
	if detail.TaskName == "" {
		detail.TaskName = task.TaskName
	}
	return detail, nil
}

// RangeCheck 是位置校验结果。
type RangeCheck struct {
	InRange bool          `json:"inRange"`
	Places  []PlaceResult `json:"places"`
	Message string        `json:"message"`
}

// PlaceResult 是单个签到点的距离信息。
type PlaceResult struct {
	Name      string  `json:"name"`
	Distance  float64 `json:"distance"`
	Radius    float64 `json:"radius"`
	InRange   bool    `json:"inRange"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// CheckRange 校验给定坐标是否落在签到点范围内。
func (d *TaskDetail) CheckRange(lon, lat float64) RangeCheck {
	if len(d.SignPlaces) == 0 {
		return RangeCheck{InRange: d.IsMalposition.Int() == 0, Message: "该任务未设置签到点，按服务端标记处理"}
	}
	result := RangeCheck{Places: make([]PlaceResult, 0, len(d.SignPlaces))}
	for _, place := range d.SignPlaces {
		dist := Haversine(lon, lat, place.Longitude.Float(), place.Latitude.Float())
		in := dist <= place.Radius.Float()
		if in {
			result.InRange = true
		}
		result.Places = append(result.Places, PlaceResult{
			Name:     firstNonEmpty(place.Address, place.PlaceName, "签到点"),
			Distance: dist, Radius: place.Radius.Float(), InRange: in,
			Latitude: place.Latitude.Float(), Longitude: place.Longitude.Float(),
		})
	}
	if result.InRange {
		result.Message = "当前坐标在签到范围内"
	} else {
		result.Message = "当前坐标不在任何签到点范围内"
	}
	return result
}

// Haversine 返回两点球面距离（米）。
func Haversine(lon1, lat1, lon2, lat2 float64) float64 {
	const earthRadius = 6371393.0
	rad := func(v float64) float64 { return v * math.Pi / 180 }
	dLon := rad(lon2 - lon1)
	dLat := rad(lat2 - lat1)
	a := math.Pow(math.Sin(dLat/2), 2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Pow(math.Sin(dLon/2), 2)
	return 2 * earthRadius * math.Asin(math.Sqrt(a))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// PickExtraAnswer 为附加题挑选答案：优先常见正向选项，否则用第一项。
func PickExtraAnswer(field ExtraField) (wid, content string, ok bool) {
	if len(field.Items) == 0 {
		return "", "", false
	}
	keywords := []string{"在校", "是", "正常", "已返校", "住校", "符合", "已归寝"}
	for _, kw := range keywords {
		for _, item := range field.Items {
			if strings.Contains(item.Content, kw) {
				return item.WID, item.Content, true
			}
		}
	}
	return field.Items[0].WID, field.Items[0].Content, true
}

// PhotoUploader 抽象照片上传，便于测试替换。
type PhotoUploader func(ctx context.Context, c *Client, host string, kind TaskKind, imagePath string) (storageKey string, preview string, err error)

// UploadPhoto 保存照片到今日校园的对象存储并返回可引用 key。
func UploadPhoto(ctx context.Context, c *Client, host string, kind TaskKind, imagePath string) (string, string, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", "", fmt.Errorf("读取照片失败：%w", err)
	}

	endpoints := EndpointsFor(kind)
	policyResp, err := c.postJSON(ctx, "https://"+host+endpoints.STS,
		map[string]string{"content-type": "application/json"},
		map[string]any{"fileType": 1})
	if err != nil {
		return "", "", fmt.Errorf("获取上传凭证失败：%w", err)
	}

	var policy struct {
		AccessID  string            `json:"accessid"`
		Host      string            `json:"host"`
		Policy    string            `json:"policy"`
		Signature string            `json:"signature"`
		FileName  string            `json:"fileName"`
		OssType   int               `json:"ossType"`
		FormData  map[string]string `json:"formData"`
	}
	if err := json.Unmarshal(policyResp.Data, &policy); err != nil {
		return "", "", fmt.Errorf("解析上传凭证失败：%w", err)
	}
	if policy.Host == "" {
		return "", "", errors.New("上传凭证缺少目标地址")
	}

	// 扩展名按实际文件走，服务端返回的 fileName 不带后缀
	ext := strings.ToLower(filepath.Ext(imagePath))
	if ext == "" {
		ext = ".jpg"
	}
	key := policy.FileName
	if key == "" {
		key = fmt.Sprintf("counselor/attendance/%d/attachment/%d", time.Now().Unix(), time.Now().UnixNano())
	}
	if !strings.HasSuffix(key, ext) {
		key += ext
	}

	// 华为 OBS 直传表单：formData 已含 key/policy/signature/AccessKeyId/x-obs-acl
	fields := map[string]string{}
	for k, v := range policy.FormData {
		fields[k] = v
	}
	if len(fields) == 0 {
		fields["key"] = key
		fields["policy"] = policy.Policy
		fields["signature"] = policy.Signature
		fields["AccessKeyId"] = policy.AccessID
		fields["x-obs-acl"] = "public-read"
	}
	fields["key"] = key

	if err := postMultipart(ctx, c, policy.Host, fields, "file", data); err != nil {
		return "", "", err
	}

	// 取回可访问 URL
	previewResp, err := c.postJSON(ctx, "https://"+host+endpoints.Preview,
		map[string]string{"content-type": "application/json"},
		map[string]any{"ossKey": key})
	if err != nil {
		return key, "", err
	}
	var previewURL string
	if err := json.Unmarshal(previewResp.Data, &previewURL); err != nil {
		previewURL = ""
	}
	return key, previewURL, nil
}

func postMultipart(ctx context.Context, c *Client, target string, fields map[string]string, fileField string, fileData []byte) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 字段顺序对 OBS 签名校验没有要求，但 key 必须与签名时一致
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile(fileField, "blob")
	if err != nil {
		return err
	}
	if _, err := part.Write(fileData); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.1; Win64; x64; rv:50.0) Gecko/20100101 Firefox/50.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("上传照片失败：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("上传照片失败（HTTP %d）：%s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// SignOptions 是一次签到所需的全部输入。
type SignOptions struct {
	Kind      TaskKind
	Task      Task
	Lon       float64
	Lat       float64
	Position  string
	Abnormal  string
	PhotoPath string
	// ResolvePhoto 在任务确实需要照片（detail.isPhoto=1）时才调用，
	// 返回本次要上传的照片绝对路径。照片池的顺序轮换挂在它后面，
	// 不需要照片的任务不会白白推进轮换。
	ResolvePhoto func() (string, bool)
	ExtraAnswers map[string]string // title -> content
	// ManualCaptcha 在自动识别不可用时把九宫格交给用户点选。
	// 只有「用户就在面板前」的执行路径（手动触发）才应该传，
	// 定时任务传 nil，否则会挂起一个永远没人处理的等待。
	ManualCaptcha ManualCaptchaFunc
}

// SignResult 是提交结果。
type SignResult struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Payload string `json:"-"`
	// CaptchaBy 记录这次验证码是谁解出来的："auto" 模型识别，
	// "manual" 用户点选。审计里要看得出人工介入过。
	CaptchaBy string `json:"captchaBy,omitempty"`
}

// Sign 执行完整签到流程：拉详情 → 组表单 → 加密 → 提交。
func (c *Client) Sign(ctx context.Context, host, tenantID string, device DeviceProfile, opts SignOptions) (*SignResult, *TaskDetail, error) {
	detail, err := c.FetchDetail(ctx, host, opts.Kind, opts.Task)
	if err != nil {
		return nil, nil, err
	}

	// 任务已经签过就不再提交：可能来自上一轮「待核对」的自动重试，
	// 也可能是学生在 App 里自己签过。详情里的 signStatus 是权威状态，
	// 重复提交不仅多余，还会让审计多出一条「成功」的糊涂账。
	if strings.TrimSpace(string(detail.SignStatus)) == "4" {
		return &SignResult{
			Success: true,
			Code:    "0",
			Message: "该任务已签到，无需重复提交",
		}, detail, nil
	}

	check := detail.CheckRange(opts.Lon, opts.Lat)

	form := map[string]any{
		"signInstanceWid": detail.SignInstanceWID,
		"longitude":       opts.Lon,
		"latitude":        opts.Lat,
		"isMalposition":   0,
		"abnormalReason":  opts.Abnormal,
		"position":        opts.Position,
		"uaIsCpadaily":    true,
		"signVersion":     "1.0.0",
	}
	if !check.InRange {
		form["isMalposition"] = 1
	}

	if detail.IsPhoto.Int() == 1 {
		photoPath := opts.PhotoPath
		if opts.ResolvePhoto != nil {
			photoPath, _ = opts.ResolvePhoto()
		}
		if photoPath == "" {
			return &SignResult{Success: false, Message: "该任务需要上传照片", Hint: "请先在账号详情的「照片」页上传照片"}, detail, nil
		}
		_, preview, err := UploadPhoto(ctx, c, host, opts.Kind, photoPath)
		if err != nil {
			return nil, detail, err
		}
		// signPhotoUrl 必须是完整可访问 URL（抓包确认是 https://wecres.../xxx.png?sig），
		// 只给 ossKey 相对路径服务端会认为照片无效。
		if preview == "" {
			return &SignResult{
				Success: false,
				Message: "照片已上传但未取到访问地址",
				Hint:    "预览接口未返回图片 URL，请稍后重试；若持续失败请检查账号登录状态",
			}, detail, nil
		}
		form["signPhotoUrl"] = preview
	} else {
		form["signPhotoUrl"] = ""
	}

	if detail.IsNeedExtra.Int() == 1 {
		items := make([]map[string]string, 0, len(detail.ExtraFields))
		for _, field := range detail.ExtraFields {
			answer := opts.ExtraAnswers[field.Title]
			wid, content := "", ""
			if answer != "" {
				for _, item := range field.Items {
					if item.Content == answer {
						wid, content = item.WID, item.Content
						break
					}
				}
			}
			if wid == "" {
				wid, content, _ = PickExtraAnswer(field)
			}
			if wid != "" {
				items = append(items, map[string]string{
					"extraFieldItemWid":   wid,
					"extraFieldItemValue": content,
				})
			}
		}
		form["extraFieldItems"] = items
	}

	if opts.Kind == KindAttendance {
		form["qrUuid"] = ""
	}

	result, err := c.submit(ctx, host, tenantID, device, opts.Kind, form, opts.ManualCaptcha)
	if err != nil {
		if ae, ok := err.(*APIError); ok {
			return &SignResult{
				Success: false, Code: ae.Code, Message: ae.Message,
				Hint: ExplainFailure(ae.Code, ae.Message),
			}, detail, nil
		}
		return nil, detail, err
	}
	return result, detail, nil
}

// submit 提交签到。
//
// 部分学校的任务要求「以图判图」验证码：通过后服务端签发一次性
// ticket，必须随提交体一起参与 AES + sign，否则被 2210060000 拒绝。
//
// 三段式，兼顾「不拖慢绝大多数学校」与「不让验证码学校白失败一次」：
//
//  1. 先问一次 checkValidation，但只给很短的超时。绝大多数学校在这里
//     就返回 validation=false，随后正常提交，不会多出一次失败提交
//     （失败提交会在学校侧留下一条记录，风控上也不干净）。
//  2. checkValidation 说需要验证码，就先换 ticket 再提交——一次成功。
//  3. checkValidation 超时/报错，或说不需要但服务端仍然拒绝（2210060000），
//     再按需补一次验证码流程并重试。这条兜底是必须的：实测存在
//     checkValidation 与提交判定不一致的部署。
func (c *Client) submit(ctx context.Context, host, tenantID string, device DeviceProfile, kind TaskKind, form map[string]any, manual ManualCaptchaFunc) (*SignResult, error) {
	if ticket, byManual, cerr, handled := c.tryCaptchaBeforeSubmit(ctx, host, kind, device.DeviceID, manual); handled {
		if ticket == "" {
			// 用户取消、或验证码没解开：不要再提交一次，避免产生无意义失败。
			return captchaAbortResult(cerr, byManual), nil
		}
		form["ticket"] = ticket
		result, err := c.submitOnce(ctx, host, tenantID, device, kind, form)
		if err != nil || result == nil {
			return result, err
		}
		// 已经用新鲜 ticket 提交过一次；再失败就不再重复整条验证码链路，
		// 把服务端结果原样交回（连续两次被拒通常不是「再解一次」能解决的）。
		result.CaptchaBy = captchaBy(byManual)
		return result, nil
	}

	result, err := c.submitOnce(ctx, host, tenantID, device, kind, form)
	if err != nil || result == nil || result.Success || !needsCaptcha(result) {
		return result, err
	}
	ticket, byManual, cerr := c.captchaTicketFor(ctx, host, kind, device.DeviceID, manual)
	if cerr != nil {
		// 用户取消手动验证：不要伪装成提交失败，给一条能看懂的收尾。
		if errors.Is(cerr, ErrManualCaptchaAborted) {
			return &SignResult{
				Success: false,
				Code:    result.Code,
				Message: cerr.Error(),
				Hint:    "本次签到未完成，需要时请重新执行",
				Payload: result.Payload,
			}, nil
		}
		// 学校侧状态自相矛盾：提交要 ticket，checkValidation 又说不需要。
		// 把服务端原始结果交回去，用户至少能看到真实原因。
		if errors.Is(cerr, errCaptchaNotRequired) {
			return result, nil
		}
		return &SignResult{
			Success: false,
			Code:    result.Code,
			Message: "验证码处理失败",
			Hint:    cerr.Error(),
			Payload: result.Payload,
		}, nil
	}
	if ticket == "" {
		// 没拿到 ticket 就不要再提交一次，把服务端原始结果交回去
		return result, nil
	}
	form["ticket"] = ticket
	retry, rerr := c.submitOnce(ctx, host, tenantID, device, kind, form)
	if rerr != nil {
		return nil, rerr
	}
	if retry != nil {
		retry.CaptchaBy = captchaBy(byManual)
	}
	return retry, nil
}

// tryCaptchaBeforeSubmit 在提交前用一次短超时的 checkValidation 探路。
//
// handled=true 表示这次已经处理过验证码：ticket 非空就带着提交；
// ticket 为空（用户取消 / 解不开）由调用方按未完成收尾。
// handled=false 表示可以直接按原样提交——不需要验证码，或探测失败。
//
// 探测失败一律降级为「直接提交」：checkValidation 只是优化，不该因为
// 它慢或抽风就阻塞签到。这条降级路径让 99% 的学校只多花一次小请求。
func (c *Client) tryCaptchaBeforeSubmit(ctx context.Context, host string, kind TaskKind, deviceID string, manual ManualCaptchaFunc) (ticket string, byManual bool, cerr error, handled bool) {
	probeCtx, cancel := context.WithTimeout(ctx, captchaProbeTimeout)
	defer cancel()

	ch, err := CheckCaptchaValidation(probeCtx, c, host, kind, deviceID)
	if err != nil || ch == nil {
		return "", false, nil, false
	}
	if !ch.Required {
		return "", false, nil, false
	}
	t, m, serr := c.solveCaptcha(ctx, host, ch, deviceID, manual)
	if serr != nil {
		return "", m, serr, true
	}
	return t, m, nil, true
}

// captchaAbortResult 是「确认要验证码、但没拿到 ticket」时的最终结果。
func captchaAbortResult(cause error, byManual bool) *SignResult {
	msg := "验证码未完成"
	hint := "本次签到未完成，需要时请重新执行"
	if cause != nil {
		msg = cause.Error()
		if errors.Is(cause, ErrManualCaptchaAborted) {
			hint = "本次签到未完成，需要时请重新执行"
		} else {
			hint = "验证码处理失败：" + cause.Error()
		}
	}
	return &SignResult{
		Success:   false,
		Message:   msg,
		Hint:      hint,
		CaptchaBy: captchaBy(byManual),
	}
}

// captchaBy 把「谁解的验证码」翻成审计字段。
func captchaBy(byManual bool) string {
	if byManual {
		return "manual"
	}
	return "auto"
}

// needsCaptcha 判断本次失败是否因为缺少验证码 ticket。
// 业务码优先；个别学校返回的 code 不稳定，message 里带「验证码」也认。
func needsCaptcha(r *SignResult) bool {
	if r == nil {
		return false
	}
	if r.Code == captchaRequiredCode {
		return true
	}
	return strings.Contains(r.Message, "验证码")
}

// submitOnce 是一次完整的提交：加密、签名、发请求、解析结果。
func (c *Client) submitOnce(ctx context.Context, host, tenantID string, device DeviceProfile, kind TaskKind, form map[string]any) (*SignResult, error) {
	endpoints := EndpointsFor(kind)

	body, err := json.Marshal(form)
	if err != nil {
		return nil, err
	}
	bodyString, err := AESEncryptCBC(string(body), FKeyCat)
	if err != nil {
		return nil, err
	}

	isIOS := device.IsIOS()
	systemName := "android"
	if isIOS {
		systemName = "iOS"
	}
	systemVersion := firstNonEmpty(device.SystemVersion, "14")
	model := firstNonEmpty(device.DeviceModel, "23127PN0CC")
	appVersion := firstNonEmpty(device.AppVersion, AppVersion)

	// 设备扩展体：Android 与 iOS 的字段名基本一致，
	// 但 iOS 额外带 cpdailyVersion，且 systemName 为大写 iOS。
	deviceInfo := map[string]any{
		"systemName":    systemName,
		"systemVersion": systemVersion,
		"model":         model,
		"deviceId":      device.DeviceID,
		"appVersion":    appVersion,
		"lon":           device.Lon,
		"lat":           device.Lat,
		"userId":        device.UserID,
	}
	if isIOS {
		deviceInfo["cpdailyVersion"] = appVersion
	}
	deviceJSON, _ := json.Marshal(deviceInfo)
	extEnc, err := DESEncrypt(string(deviceJSON), desKeyExtension)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"User-Agent":        device.UserAgent(),
		"Content-Type":      "application/json",
		"Host":              host,
		"CpdailyStandAlone": "0",
		"CpdailyClientType": "CPDAILY",
		"deviceType":        "1",
		"Cpdaily-Extension": extEnc,
		"Accept":            "application/json, text/plain, */*",
		"tenantId":          firstNonEmpty(tenantID, DefaultTenantID),
	}

	if isIOS {
		// iOS 客户端只发 Cpdaily-Extension，不发 CpdailyInfo / extension / sign，
		// Content-Type 也不带 charset。deviceType 固定为 2。
		headers["deviceType"] = "2"
		headers["Accept"] = "*/*"
		headers["Accept-Language"] = "zh-CN,zh-Hans;q=0.9"
		headers["CacheTimeValue"] = "0"
		headers["Accept-Encoding"] = "gzip, deflate"
		headers["Connection"] = "keep-alive"
		if device.WisDeviceID != "" {
			headers["wisDeviceId"] = device.WisDeviceID
		}
		// iOS 原生客户端手动拼 Cookie，必须显式带上 MOD_AUTH_CAS
		headers["Cookie"] = c.iosCookieHeader(tenantID)
	} else {
		headers["Content-Type"] = "application/json; charset=utf-8"
		headers["Accept-Encoding"] = "gzip"
		headers["Connection"] = "Keep-Alive"
		headers["extension"] = "1"
		headers["sign"] = "1"
		headers["clientType"] = "cpdaily_student"
		infoEnc, err := DESEncrypt(string(deviceJSON), desKeyInfo)
		if err != nil {
			return nil, err
		}
		headers["CpdailyInfo"] = infoEnc
	}

	// 账号令牌头已由 client.do() 统一注入，这里不再重复。

	// 提交体：设备字段 + bodyString + 版本标记 + sign。
	// 外层必须同时带 lon/lat，服务端会用它们做一次快速校验，
	// 缺失时报「签到地点纬度不能为空」。
	payload := map[string]any{}
	for k, v := range deviceInfo {
		if k == "cpdailyVersion" {
			continue
		}
		payload[k] = v
	}
	payload["bodyString"] = bodyString
	payload["version"] = "first_v4"
	payload["calVersion"] = "firstv"
	// sign = md5(urlencode(表单 + bodyString) & key)
	signForm := map[string]any{}
	for k, v := range form {
		signForm[k] = v
	}
	signForm["bodyString"] = bodyString
	payload["sign"] = MD5Hex(urlEncodeSorted(signForm) + "&" + FKeyCat)

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodPost, "https://"+host+endpoints.Submit, headers, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("提交失败：%w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	var parsed struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	code := strings.Trim(string(parsed.Code), `"`)

	// 服务端在会话失效时会直接返回登录页 HTML 而不是 JSON，
	// 这种情况给出明确提示，避免用户以为是设备或参数问题。
	if strings.HasPrefix(strings.TrimSpace(string(respBody)), "<") {
		return &SignResult{
			Success: false,
			Code:    "session_expired",
			Message: "登录已失效",
			Hint:    "学校会话已过期（返回登录页），请在账号里重新登录后再执行",
			Payload: truncate(string(respBody), 200),
		}, nil
	}

	result := &SignResult{Code: code, Message: parsed.Message, Payload: string(respBody)}

	// 成功判定：查寝提交成功时服务端返回 {"code":"0","message":"SUCCESS",...}，
	// 响应里没有 datas 字段（与签到接口不同）。因此只认 code 与 message，
	// 不能再要求 datas.signedStuInfo 存在。
	if code == "0" ||
		strings.EqualFold(strings.TrimSpace(parsed.Message), "SUCCESS") ||
		strings.Contains(parsed.Message, "成功") {
		result.Success = true
	}
	if !result.Success {
		result.Hint = ExplainFailure(code, parsed.Message)
		if result.Hint == "" || result.Message == "" {
			// 保留服务端原文，便于排查
			result.Hint = "服务端返回：" + truncate(string(respBody), 200)
		}
	}
	return result, nil
}

// iosCookieHeader 拼 iOS 原生客户端要求的 Cookie 串。
//
// 实测：iOS 提交必须显式带上 MOD_AUTH_CAS，否则服务端直接返回登录页 HTML。
func (c *Client) iosCookieHeader(tenantID string) string {
	parts := []string{"clientType=cpdaily_student", "standAlone=0"}
	if tenantID != "" {
		parts = append(parts, "tenantId="+tenantID)
	}
	if v := c.cookies["sessionToken"]; v != "" {
		parts = append(parts, "sessionToken="+v)
	}
	if v := c.cookies["MOD_AUTH_CAS"]; v != "" {
		parts = append(parts, "MOD_AUTH_CAS="+v)
	}
	return strings.Join(parts, "; ")
}

// urlEncodeSorted 按 key 排序后拼成 a=1&b=2 形式（值用 fmt.Sprint）。
// 服务端对 sign 的校验较宽松，但格式需要与客户端一致。
func urlEncodeSorted(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(fmt.Sprint(m[k]))
	}
	return b.String()
}

// ExplainFailure 把服务端错误翻译成可操作的建议。
func ExplainFailure(code, message string) string {
	switch {
	case code == "3" || strings.Contains(message, "更换手机") || strings.Contains(message, "已被") || strings.Contains(message, "授权"):
		return "设备未授权：当前设备 ID 与该账号绑定的设备不一致。请用「设备码」扫码，在你平时打卡的那台手机上获取设备 ID；" +
			"或先用那台手机在今日校园正常签到一次，也可联系辅导员在「辅导猫 → 手机授权管理」中授权"
	case strings.Contains(message, "版本过低"):
		return "服务端拒绝了本次请求（提示版本过低），通常是加密密钥或客户端版本已轮换，请更新服务端配置"
	case code == captchaRequiredCode || strings.Contains(message, "验证码"):
		return "该校签到需要图形验证码。手动执行时面板会弹窗让你选出目标图片；" +
			"定时任务请先在管理员设置里配置识别服务，否则到点只能失败"
	case code == "4":
		return "服务端无法解析提交内容，请检查账号、设备信息与签到点是否匹配"
	case strings.Contains(message, "范围"):
		return "坐标不在签到范围内：请调整定位到签到点半径内后重试"
	case strings.Contains(message, "已签到"):
		return "该任务已完成，无需重复提交"
	}
	if message != "" {
		return message
	}
	return fmt.Sprintf("提交失败（code=%s）", code)
}
