package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"qjrxy/internal/cpdaily"
	"qjrxy/internal/events"
	"qjrxy/internal/store"
)

// RunOptions 控制一次任务执行。
type RunOptions struct {
	UserID    int64
	AccountID int64
	TaskID    int64 // 0 表示自动挑选第一个待办
	Kind      string
	Source    string // manual / auto
	PlanID    int64
}

// RunOutcome 是执行结果。
type RunOutcome struct {
	LogID   int64  `json:"logId"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Task    string `json:"taskName"`
}

// RunTask 执行一次签到/查寝，并写入执行记录。
func (s *Service) RunTask(ctx context.Context, opts RunOptions) (*RunOutcome, error) {
	account, err := s.db.AccountByID(ctx, opts.UserID, opts.AccountID)
	if err != nil {
		return nil, err
	}
	// 会话缺失或已过期时，先用保存的密码自动重新登录，
	// 避免「隔了几天再执行就报会话过期」需要手动处理。
	if !account.HasCookie() || strings.EqualFold(account.LoginStatus, "expired") {
		if !hasCredentials(account) {
			return nil, errors.New("账号尚未登录，请先在账号页完成账密登录")
		}
		if account, err = s.AutoRelogin(ctx, account); err != nil {
			return nil, fmt.Errorf("自动登录失败：%w", err)
		}
	}
	// 设备 ID 为空时直接拒绝：拿空值提交等于用一台陌生设备去撞服务端，
	// 会被判定为「更换手机」并累加风控计数，必须先在账号里绑定真实设备。
	if strings.TrimSpace(account.DeviceID) == "" {
		return nil, errors.New("尚未绑定设备信息：请到账号页扫码获取，用你平时打卡的那台手机扫二维码")
	}
	// 本项目只处理查寝（宿舍考勤）任务。
	// 二维码签到类任务涉及范围/时效差异较大，且容易产生无效提交，故不执行。
	if opts.Kind != "" && opts.Kind != string(cpdaily.KindAttendance) {
		return nil, errors.New("当前仅支持查寝任务，其他类型任务不会执行")
	}

	task, err := s.pickTask(ctx, account, opts)
	if err != nil {
		return nil, err
	}

	// 检查任务是否在可签到时段内。
	// 服务端在「任务还没开始」时不会报错，而是返回 code=0 但 signedStuInfo 为空，
	// 看起来像成功、实际没签上。提前拦住可以避免这类无效提交。
	if err := checkTaskWindow(task); err != nil {
		return nil, err
	}

	traceID := NewTraceID()
	// 注入追踪 ID：诊断日志用它把同一次任务的全部 HTTP 请求串起来。
	ctx = cpdaily.WithTraceID(ctx, traceID)
	startedAt := time.Now()
	logID, err := s.db.CreateLog(ctx, &store.Log{
		UserID:    opts.UserID,
		AccountID: account.ID,
		PlanID:    opts.PlanID,
		TraceID:   traceID,
		TaskName:  task.TaskName,
		Kind:      task.Kind,
		Source:    defaultIfEmpty(opts.Source, "manual"),
		Status:    store.StatusExecuting,
		Message:   "正在提交",
	})
	if err != nil {
		return nil, err
	}
	// 从落库那一刻起，这次执行在审计页就该以「执行中」出现；
	// 提交可能耗时几十秒，不该等接口返回才让用户看到。
	defer s.notify(opts.UserID, events.TopicLogs)

	photoPath := s.PhotoAbsPath(account.PhotoPath)
	hasPhoto := photoPath != "" && fileExists(photoPath)

	client := s.clientFor(account)
	// 坐标加入 10 米以内的随机偏移，避免每天同一坐标值过于规律；
	// 偏移量远小于签到点半径（通常 300~1500 米），不影响范围判定。
	lon, lat := jitterCoord(account.Lon, account.Lat)
	device := deviceProfile(account, lon, lat)
	signOpts := cpdaily.SignOptions{
		Kind: cpdaily.TaskKind(task.Kind),
		Task: cpdaily.Task{
			Kind:            cpdaily.TaskKind(task.Kind),
			SignInstanceWID: task.SignInstanceWID,
			SignWID:         task.SignWID,
			TaskName:        task.TaskName,
		},
		Lon:         lon,
		Lat:         lat,
		Position:    account.Address,
		Abnormal:    account.Abnormal,
		PhotoPath:   photoPath,
		UploadPhoto: hasPhoto,
	}
	// 手动执行时允许「自动识别失败 → 弹窗让用户点」。
	// 定时任务不注入：那个时间点通常没人看屏幕，等满 100 秒只是白占资源。
	if defaultIfEmpty(opts.Source, "manual") == "manual" {
		signOpts.ManualCaptcha = s.manualCaptcha(opts.UserID, account.ID)
	}

	result, detail, err := client.Sign(ctx, account.Host, account.TenantID, device, signOpts)
	// 提交时才发现会话失效：自动重登一次并重试，用户无感知。
	if err == nil && !result.Success && hasCredentials(account) && cpdaily.IsAuthExpired(errors.New(result.Message)) {
		if updated, reErr := s.AutoRelogin(ctx, account); reErr == nil {
			account = updated
			client = s.clientFor(account)
			result, detail, err = client.Sign(ctx, account.Host, account.TenantID, device, signOpts)
		}
	}
	if err == nil {
		// 学校返回的文案可能夹带 HTML（实测「手机授权管理」提示里嵌了 <span>），
		// 原样入库会在审计页和通知里显示原始标签。
		result.Message = sanitizeSchoolMessage(result.Message)
		result.Hint = sanitizeSchoolMessage(result.Hint)
	}
	if err != nil {
		detailJSON := marshalDetail(map[string]any{"error": err.Error()})
		_ = s.db.UpdateLog(ctx, logID, store.StatusPending, err.Error(), detailJSON)
		return &RunOutcome{LogID: logID, Status: store.StatusPending, Message: err.Error(),
			Task: task.TaskName}, nil
	}

	detailJSON := marshalDetail(buildLogDetail(detail, result, account, lon, lat, task, traceID, time.Since(startedAt)))
	if result.Success {
		// 提交成功说明会话有效；顺手把这一轮拿到的续期 cookie 落库，
		// 免得到下次重建 client 时又退回登录时那份旧票。
		_ = s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(client.Cookies()), "authorized")
		// 事后核对：服务端在「任务还没到签到时间」时也会返回 code=0 SUCCESS，
		// 靠响应字段无法区分。这里回查一次任务列表，确认该任务确实从
		//「未签到」里消失了，才算真成功。
		confirmed, checkErr := s.confirmSigned(ctx, account, task)
		if checkErr == nil && !confirmed {
			_ = s.db.UpdateLog(ctx, logID, store.StatusPending,
				"服务端返回成功，但任务仍在待办列表中", detailJSON)
			return &RunOutcome{
				LogID:   logID,
				Status:  store.StatusPending,
				Message: "服务端返回成功，但任务仍在待办列表中",
				Hint:    "可能是当前不在该任务的签到时段。请核对任务时间，到时间后再执行",
				Task:    task.TaskName,
			}, nil
		}

		_ = s.db.UpdateLog(ctx, logID, store.StatusSuccess, defaultIfEmpty(result.Message, "已完成"), detailJSON)
		_ = s.db.MarkTaskDone(ctx, account.ID, task.Kind, task.SignInstanceWID)
		// 任务变成「已处理」、会话 cookie 也已续期，列表与账号卡片一起刷新。
		s.notify(opts.UserID, events.TopicTasks, events.TopicAccounts)
		return &RunOutcome{LogID: logID, Status: store.StatusSuccess,
			Message: defaultIfEmpty(result.Message, "已完成"), Task: task.TaskName}, nil
	}

	status := store.StatusFailed
	if isVerification(result.Code, result.Message) {
		status = store.StatusPending
	}
	_ = s.db.UpdateLog(ctx, logID, status, defaultIfEmpty(result.Message, "提交失败"), detailJSON)
	if cpdaily.IsAuthExpired(errors.New(result.Message)) {
		_ = s.db.UpdateAccountSession(ctx, account.ID, encodeCookies(client.Cookies()), "expired")
		s.notify(opts.UserID, events.TopicAccounts)
	}
	hint := result.Hint
	if IsDeviceAuthRequired(result.Code, result.Message) {
		// 确定性权限拒绝：10 分钟重试注定失败，还会给学校风控累积失败记录。
		// 调度器据此停止当天自动重试，等待人工授权后手动恢复。
		hint = defaultIfEmpty(hint, "请先联系辅导员在辅导猫「手机授权管理」中授权本机，处理后手动执行或重置计划")
	}
	return &RunOutcome{LogID: logID, Status: status, Message: result.Message,
		Hint: hint, Task: task.TaskName}, nil
}

// TaskWindow 是一个任务的签到时间窗口。
type TaskWindow struct {
	TaskName string    `json:"taskName"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	// Ready 表示此刻已进入可提交时段。
	Ready bool `json:"ready"`
}

// NextTaskWindow 从本地任务缓存算出「下一个值得关注的签到窗口」。
//
// 自动计划的核心语义是「已知窗口 → 窗口内尽快执行」，而不是反复同步接口
// 试探任务什么时候开始。窗口信息在同步任务时就已经落在本地缓存里，
// 调度器据此算出精确的触发时刻，到点前发一次请求即可。
//
// 返回值 rules：
//   - 已进入窗口 → Ready=true，应立即执行
//   - 还没到窗口 → Ready=false，Start 是应当唤醒的时间
//   - 没有可用窗口（无任务 / 已过期 / 已完成 / 无时间字段）→ ok=false
func (s *Service) NextTaskWindow(ctx context.Context, accountID int64, kind string) (TaskWindow, bool) {
	cached, err := s.db.TasksByAccount(ctx, accountID)
	if err != nil {
		return TaskWindow{}, false
	}
	now := time.Now()
	want := strings.TrimSpace(kind)

	var best TaskWindow
	var found bool
	for i := range cached {
		t := &cached[i]
		if t.Status == "done" || t.Status == "leave" {
			continue
		}
		if want != "" && t.Kind != want {
			continue
		}
		st := strings.TrimSpace(t.SignStatus)
		if st == "3" || st == "4" {
			continue // 已过期 / 已签到
		}

		w := parseTaskWindow(t)
		if !w.ok {
			// 没有时间字段，无法算出窗口：
			//   - signStatus=2（学校明确说未到时间）→ 不可执行，且没有可唤醒的时刻
			//   - 其它（如自由签到）→ 交给服务端判定
			if st == "2" {
				continue
			}
			return TaskWindow{TaskName: t.TaskName, Ready: true}, true
		}
		if now.After(w.end) {
			continue // 该窗口已关闭
		}
		candidate := TaskWindow{
			TaskName: t.TaskName,
			Start:    w.start,
			End:      w.end,
			Ready:    !now.Before(w.start),
		}
		// 优先返回已就绪的；否则返回最早开启的那个
		if candidate.Ready {
			return candidate, true
		}
		if !found || candidate.Start.Before(best.Start) {
			best = candidate
			found = true
		}
	}
	return best, found
}

// HasRunnableTask 判断账号当前是否存在可提交的指定类型任务。
//
// 供调度器在真正提交前做前置判断：任务还没到签到时间时直接等下一轮，
// 不要发起一次必然被拒的请求（既浪费额度也会污染执行记录）。
func (s *Service) HasRunnableTask(ctx context.Context, accountID int64, kind string) bool {
	w, ok := s.NextTaskWindow(ctx, accountID, kind)
	return ok && w.Ready
}

// pickTask 选择要执行的任务。
func (s *Service) pickTask(ctx context.Context, account *store.Account, opts RunOptions) (*store.Task, error) {
	if opts.TaskID > 0 {
		task, err := s.db.TaskByID(ctx, opts.UserID, opts.TaskID)
		if err != nil {
			return nil, err
		}
		if task.AccountID != account.ID {
			return nil, errors.New("任务不属于该账号")
		}
		return task, nil
	}

	cached, err := s.db.TasksByAccount(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if len(cached) == 0 {
		if _, err := s.SyncTasks(ctx, opts.UserID, account.ID); err != nil {
			return nil, err
		}
		if cached, err = s.db.TasksByAccount(ctx, account.ID); err != nil {
			return nil, err
		}
	}
	for i := range cached {
		t := cached[i]
		// 已处理、已请假的任务都不能再提交：
		// 给请假任务打卡会产生错误的考勤记录。
		if t.Status == "done" || t.Status == "leave" {
			continue
		}
		// 只挑查寝任务
		if t.Kind != string(cpdaily.KindAttendance) {
			continue
		}
		if opts.Kind != "" && t.Kind != opts.Kind {
			continue
		}
		// 只在可签到时段内挑选。「未到时间」的任务必须跳过，
		// 否则会把一个还不允许提交的任务硬发出去，被服务端拒绝为「查寝未开始」。
		if err := checkTaskWindow(&t); err != nil {
			continue
		}
		return &t, nil
	}
	return nil, errors.New(noRunnableTaskMessage(cached))
}

// noRunnableTaskMessage 在没有可执行任务时给出可区分的提示。
//
// 「未到时间」和「没有任务」是两种完全不同的情况：前者应等待，
// 后者说明任务还没下发或已处理完。混淆会让计划执行记录失去意义。
func noRunnableTaskMessage(cached []store.Task) string {
	var notYet, expired, done int
	for _, t := range cached {
		if t.Kind != string(cpdaily.KindAttendance) {
			continue
		}
		switch strings.TrimSpace(t.SignStatus) {
		case "2":
			notYet++
		case "3":
			expired++
		case "4":
			done++
		}
	}
	switch {
	case notYet > 0:
		return "查寝任务还未到签到时间，等待下一个时间窗口"
	case expired > 0:
		return "查寝任务已过签到时间"
	case done > 0:
		return "今日查寝任务已完成"
	}
	return "当前没有待处理的查寝任务"
}

// confirmSigned 回查任务列表，确认任务是否真的完成了。
//
// 返回 (true, nil) 表示已不在待办中（成功）；
// (false, nil) 表示仍在待办中（未真正签到）；
// 查询失败时返回 err，调用方应视为「无法确认」而不是失败。
func (s *Service) confirmSigned(ctx context.Context, account *store.Account, task *store.Task) (bool, error) {
	client := s.clientFor(account)
	list, err := client.ListAllTasks(ctx, account.Host, []cpdaily.TaskKind{cpdaily.KindAttendance})
	if err != nil {
		return false, err
	}
	for _, t := range list {
		if t.SignInstanceWID == task.SignInstanceWID {
			return false, nil // 还在待办里，说明没签上
		}
	}
	return true, nil
}

// buildLogDetail 组装可回溯的执行详情。
func buildLogDetail(detail *cpdaily.TaskDetail, result *cpdaily.SignResult, account *store.Account,
	lon, lat float64, task *store.Task, traceID string, latency time.Duration) map[string]any {
	payload := map[string]any{
		"code":       result.Code,
		"message":    result.Message,
		"lon":        lon,
		"lat":        lat,
		"address":    account.Address,
		"device":     account.DeviceID,
		"traceId":    traceID,
		"latencyMs":  latency.Milliseconds(),
		"signDate":   task.SignDate,
		"beginClock": task.BeginClock,
		"endClock":   task.EndClock,
	}
	// 人工介入要留痕：事后看到「验证码通过」时，能分清是模型认出来的
	// 还是用户自己点的。
	if result.CaptchaBy != "" {
		payload["captchaBy"] = result.CaptchaBy
	}
	if detail != nil {
		payload["taskName"] = detail.TaskName
		payload["isPhoto"] = detail.IsPhoto.Int() == 1
		payload["isNeedExtra"] = detail.IsNeedExtra.Int() == 1
		check := detail.CheckRange(lon, lat)
		payload["inRange"] = check.InRange
		payload["places"] = check.Places
		// 取最近的签到点作为基准，记录实际提交坐标相对它的偏移
		if len(check.Places) > 0 {
			minDist := check.Places[0].Distance
			payload["geoOffsetM"] = minDist
			payload["geoBase"] = check.Places[0].Name
		}
	}
	return payload
}

func marshalDetail(v any) string {
	buf, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(buf)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sanitizeSchoolMessage 清理学校返回文案中的 HTML 标签与多余空白。
// 服务端偶尔在提示里内嵌样式标签（如红色的「手机授权管理」），原样透传
// 会让前端显示出一堆原始标签。
func sanitizeSchoolMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if !strings.Contains(msg, "<") {
		return msg
	}
	tag := regexp.MustCompile(`<[^>]*>`)
	msg = tag.ReplaceAllString(msg, "")
	// 去标签后可能留下连续空格
	ws := regexp.MustCompile(`\s{2,}`)
	return strings.TrimSpace(ws.ReplaceAllString(msg, " "))
}

// IsDeviceAuthRequired 识别「设备需辅导员授权」类确定性拒绝（学校 code=3）。
//
// 这类错误重试没有意义——必须由辅导员在辅导猫后台授权后才可能成功；
// 调度器据此停止当天自动重试，避免给学校风控累积大量失败提交记录。
func IsDeviceAuthRequired(code, message string) bool {
	joined := code + " " + message
	for _, kw := range []string{"更换手机", "手机授权", "辅导猫", "授权使用该手机"} {
		if strings.Contains(joined, kw) {
			return true
		}
	}
	return false
}

// isVerification 判断失败是否属于"需要人工核对"而非确定失败。
func isVerification(code, message string) bool {
	joined := code + " " + message
	for _, kw := range []string{"人脸", "验证", "二维码", "扫码", "定位异常", "暂未"} {
		if strings.Contains(joined, kw) {
			return true
		}
	}
	return false
}

// checkTaskWindow 校验任务是否处于可签到时段。
//
// 优先依据列表接口返回的 signStatus：
//   - "2" 未到签到时间
//   - "3" 已过期
//   - "4" 已签到
//
// signStatus 为空时（部分学校/任务不下发该字段，例如按 signDate + beginClock
// 定义的周期性查寝），用任务自带的时间字段自行判断，否则会把「21:00 才开始」
// 的任务在 16:00 就显示成可执行，提交后被服务端拒绝为「查寝未开始」。
func checkTaskWindow(task *store.Task) error {
	switch strings.TrimSpace(task.SignStatus) {
	case "2":
		when := ""
		if task.SignDate != "" {
			when = "（" + task.SignDate
			if task.BeginClock != "" {
				when += " " + task.BeginClock + "-" + task.EndClock
			}
			when += "）"
		}
		return fmt.Errorf("「%s」还没到签到时间%s，请到时间后再执行", task.TaskName, when)
	case "3":
		return fmt.Errorf("「%s」已过签到时间，无法补签", task.TaskName)
	case "4":
		return fmt.Errorf("「%s」已签到，无需重复执行", task.TaskName)
	}

	// signStatus 为空：用任务自带的时间字段自行判断窗口。
	return checkTaskWindowByClock(task, time.Now())
}

// clockWindow 描述任务由 signDate + beginClock~endClock 定义的签到时段。
type clockWindow struct {
	start time.Time
	end   time.Time
	ok    bool
}

// parseTaskWindow 解析任务的日期与时段。
//
// signDate 形如 "2026-11-03" 或 "2026-11-03 (周二)"，beginClock/endClock 形如 "21:00"。
// 时段可能跨天（如 23:30-00:30），此时把结束时间推到次日。
func parseTaskWindow(task *store.Task) clockWindow {
	if task == nil {
		return clockWindow{}
	}
	dateRaw := strings.TrimSpace(task.SignDate)
	if i := strings.IndexAny(dateRaw, " (（"); i > 0 {
		dateRaw = dateRaw[:i]
	}
	if dateRaw == "" || task.BeginClock == "" || task.EndClock == "" {
		return clockWindow{}
	}
	day, err := time.ParseInLocation("2006-01-02", dateRaw, time.Local)
	if err != nil {
		return clockWindow{}
	}
	start, err1 := time.ParseInLocation("2006-01-02 15:04", dateRaw+" "+task.BeginClock, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02 15:04", dateRaw+" "+task.EndClock, time.Local)
	if err1 != nil || err2 != nil {
		return clockWindow{}
	}
	if !end.After(start) {
		end = end.AddDate(0, 0, 1) // 跨天时段
	}
	_ = day
	return clockWindow{start: start, end: end, ok: true}
}

// checkTaskWindowByClock 在没有 signStatus 时按时段判断任务能否提交。
//
// 返回的「还没到签到时间」文案与 signStatus=2 一致，
// 调度器据此识别为「等待窗口」而不消耗当天的执行机会。
func checkTaskWindowByClock(task *store.Task, now time.Time) error {
	w := parseTaskWindow(task)
	if !w.ok {
		return nil // 没有可解析的时间信息，交给服务端判定
	}
	if now.Before(w.start) {
		return fmt.Errorf("「%s」还没到签到时间（%s-%s），请到时间后再执行",
			task.TaskName, task.BeginClock, task.EndClock)
	}
	if now.After(w.end) {
		return fmt.Errorf("「%s」已过签到时间（%s-%s），无法补签",
			task.TaskName, task.BeginClock, task.EndClock)
	}
	return nil
}

// jitterCoord 在原始坐标附近 10 米内随机取一个点。
//
// 目的不是规避定位校验，而是避免长期使用完全相同的坐标值，
// 让提交看起来更接近真实用户的手动定位。10 米仍远小于签到点半径，
// 不会导致范围判定失败；坐标为 0（未设置）时不偏移。
func jitterCoord(lon, lat float64) (float64, float64) {
	if lon == 0 && lat == 0 {
		return lon, lat
	}
	const (
		maxOffsetM  = 10.0
		earthRadius = 6371393.0
	)
	// 圆盘内均匀采样：半径开方，避免点聚集在圆心。
	r := maxOffsetM * math.Sqrt(rand.Float64())
	theta := rand.Float64() * 2 * math.Pi
	metersPerDeg := earthRadius * math.Pi / 180
	cosLat := math.Cos(lat * math.Pi / 180)
	if math.Abs(cosLat) < 1e-6 {
		cosLat = 1e-6 // 极点附近经度收缩，避免除零
	}
	dLat := r * math.Cos(theta) / metersPerDeg
	dLon := r * math.Sin(theta) / (metersPerDeg * cosLat)
	return lon + dLon, lat + dLat
}

// TodayLabel 返回用于展示的当前时间文本。
func TodayLabel(t time.Time) string {
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return fmt.Sprintf("%d月%d日%s", int(t.Month()), t.Day(), weekdays[int(t.Weekday())])
}

// NewTraceID 生成一次执行的流水号，格式：QJ-YYYYMMDD-XXXXXX。
//
// 用于在审计日志里定位单次执行，前端表格与链路详情都以它为主键展示。
func NewTraceID() string {
	const charset = "0123456789ABCDEF"
	now := time.Now()
	buf := make([]byte, 6)
	for i := range buf {
		buf[i] = charset[rand.Intn(len(charset))]
	}
	return fmt.Sprintf("QJ-%s-%s", now.Format("20060102"), string(buf))
}
