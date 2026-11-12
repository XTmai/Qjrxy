package scheduler

import (
	"errors"
	"testing"
	"time"

	"qjrxy/internal/store"
)

func TestParseClock(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"00:00", 0, true},
		{"23:59", 23*60 + 59, true},
		{"20:05", 20*60 + 5, true},
		{"24:00", 0, false},
		{"12:60", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseClock(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseClock(%q) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestInWindow(t *testing.T) {
	at := func(h, m int) time.Time {
		return time.Date(2026, 11, 1, h, m, 0, 0, time.Local)
	}
	cases := []struct {
		now        time.Time
		start, end string
		want       bool
	}{
		{at(20, 30), "20:00", "23:30", true},
		{at(19, 59), "20:00", "23:30", false},
		{at(23, 31), "20:00", "23:30", false},
		// 跨天区间
		{at(23, 30), "22:00", "02:00", true},
		{at(1, 0), "22:00", "02:00", true},
		{at(12, 0), "22:00", "02:00", false},
		// 非法配置时不限制
		{at(12, 0), "bad", "worse", true},
	}
	for _, c := range cases {
		if got := inWindow(c.now, c.start, c.end); got != c.want {
			t.Errorf("inWindow(%s, %s-%s) = %v want %v",
				c.now.Format("15:04"), c.start, c.end, got, c.want)
		}
	}
}

func TestShouldRun(t *testing.T) {
	// 2026-11-01 是周日 -> 星期索引 6
	sunday := time.Date(2026, 11, 1, 20, 0, 0, 0, time.Local)
	monday := time.Date(2026, 11, 2, 20, 0, 0, 0, time.Local)

	base := store.Plan{Enabled: true, Mode: "school", Days: []int{6}}

	if !shouldRun(base, sunday) {
		t.Error("周日 + 选中周日 应执行")
	}
	if shouldRun(base, monday) {
		t.Error("周一不在执行日期内，不应执行")
	}

	// 已停用
	disabled := base
	disabled.Enabled = false
	if shouldRun(disabled, sunday) {
		t.Error("停用计划不应执行")
	}

	// 当天已执行
	ran := base
	ran.LastRunAt = "2026-11-01T20:00:00+08:00"
	if shouldRun(ran, sunday) {
		t.Error("当天已执行过不应重复")
	}

	// 昨天执行过可以再跑
	ranYesterday := base
	ranYesterday.LastRunAt = "2026-10-31T20:00:00+08:00"
	if !shouldRun(ranYesterday, sunday) {
		t.Error("昨天执行过不应阻止今天执行")
	}

	// 自定义时段外不执行
	custom := store.Plan{Enabled: true, Mode: "custom", Days: []int{6},
		WindowStart: "21:00", WindowEnd: "23:00"}
	if shouldRun(custom, sunday) {
		t.Error("20:00 不在 21:00-23:00 内，不应执行")
	}
	custom.WindowStart, custom.WindowEnd = "19:00", "21:00"
	if !shouldRun(custom, sunday) {
		t.Error("20:00 应在 19:00-21:00 内")
	}
}

// TestIsWaitingForWindow 校验「未到签到时间」不会被当成当天已执行。
//
// 回归：计划若在任务还没到时间时触发一次，曾经会把 last_run_at 写死，
// 导致当天真正到时间后不再执行。
func TestIsWaitingForWindow(t *testing.T) {
	waiting := []string{
		"「晚间查寝」还没到签到时间（2026-11-03 22:00-22:30），请到时间后再执行",
		"查寝任务还未到签到时间，等待下一个时间窗口",
	}
	for _, msg := range waiting {
		if !isWaitingForWindow(errors.New(msg)) {
			t.Errorf("应判定为等待窗口：%s", msg)
		}
	}

	notWaiting := []string{
		"「晚间查寝」已过签到时间，无法补签",
		"账号尚未登录，请先在账号页完成账密登录",
		"今日查寝任务已完成",
		"网络请求失败",
	}
	for _, msg := range notWaiting {
		if isWaitingForWindow(errors.New(msg)) {
			t.Errorf("不应判定为等待窗口：%s", msg)
		}
	}
	if isWaitingForWindow(nil) {
		t.Error("nil 不应判定为等待窗口")
	}
}

// TestInProbeWindow 校验探测只在活跃时段内进行。
//
// 凌晨到清晨探测既无收益（查寝不会在该时段下发），
// 又会给账号带来非真人的请求时间分布。
func TestInProbeWindow(t *testing.T) {
	allowed := []int{7, 8, 11, 12, 16, 18, 20, 22}
	for _, h := range allowed {
		tm := time.Date(2026, 11, 3, h, 30, 0, 0, time.Local)
		if !inProbeWindow(tm) {
			t.Errorf("%d 点应在探测时段内", h)
		}
	}
	blocked := []int{0, 3, 6, 9, 10, 14, 15}
	for _, h := range blocked {
		tm := time.Date(2026, 11, 3, h, 30, 0, 0, time.Local)
		if inProbeWindow(tm) {
			t.Errorf("%d 点不应在探测时段内", h)
		}
	}
}

// TestShouldProbeNow 校验探测节流：活跃时段内每小时最多一次。
func TestShouldProbeNow(t *testing.T) {
	base := time.Date(2026, 11, 3, 20, 0, 0, 0, time.Local)

	// 首次探测（无历史）直接放行
	if !shouldProbeNow(base, time.Time{}) {
		t.Error("首次探测应放行")
	}
	// 20 分钟前刚探测过 → 跳过
	if shouldProbeNow(base, base.Add(-20*time.Minute)) {
		t.Error("20 分钟内不应重复探测")
	}
	// 一小时前探测过 → 放行
	if !shouldProbeNow(base, base.Add(-time.Hour)) {
		t.Error("超过最小间隔应放行")
	}
	// 非活跃时段一律不放行，即使从未探测过
	night := time.Date(2026, 11, 3, 3, 0, 0, 0, time.Local)
	if shouldProbeNow(night, time.Time{}) {
		t.Error("凌晨不应探测")
	}
}

// TestProbeFrequencyEstimate 估算每日探测次数，确保不产生机器特征。
func TestProbeFrequencyEstimate(t *testing.T) {
	// 按 probeWindows 覆盖的总小时数除以最小间隔
	var hours int
	for _, w := range probeWindows {
		hours += w[1] - w[0]
	}
	perDay := hours
	if perDay > 24 {
		t.Fatalf("探测时段配置异常：%d 小时", hours)
	}
	if perDay > 20 {
		t.Errorf("每日探测上限 %d 次偏高，可能形成机器特征（建议 ≤ 20）", perDay)
	}
	if perDay < 5 {
		t.Errorf("每日探测上限 %d 次偏低，可能错过任务下发", perDay)
	}
}

// TestClaimAttemptThrottles 校验同一计划不会被每 30 秒重复尝试。
//
// 回归：计划刚创建、任务还没到签到时间时，若每次都发起同步+提交，
// 一天会产生数千次无效请求（也容易被判定为异常行为）。
func TestClaimAttemptThrottles(t *testing.T) {
	s := &Scheduler{attemptAt: map[int64]time.Time{}}
	base := time.Date(2026, 11, 3, 16, 19, 0, 0, time.Local)

	// 首次允许
	if !s.claimAttempt(1, base) {
		t.Fatal("首次尝试应被允许")
	}
	// 10 分钟内的后续触发全部拒绝
	for _, delta := range []time.Duration{30 * time.Second, 2 * time.Minute, 9 * time.Minute} {
		if s.claimAttempt(1, base.Add(delta)) {
			t.Errorf("%v 后的尝试应被节流拒绝", delta)
		}
	}
	// 超过间隔后允许
	if !s.claimAttempt(1, base.Add(attemptGap+time.Second)) {
		t.Error("超过最小间隔后应允许尝试")
	}
	// 不同计划互不影响
	if !s.claimAttempt(2, base.Add(time.Minute)) {
		t.Error("不同计划不应互相节流")
	}
}

// TestShouldWake 校验窗口驱动：未到唤醒时刻的计划不产生任何请求。
//
// 这是「已知窗口 → 窗口内尽快执行」的实现核心。相比每 10 分钟同步接口
// 试探任务是否开始，直接按任务窗口算出唤醒时刻，请求量降到每窗口一次。
func TestShouldWake(t *testing.T) {
	s := &Scheduler{wakeAt: map[int64]time.Time{}}
	windowStart := time.Date(2026, 11, 3, 21, 0, 0, 0, time.Local)

	// 未设定唤醒时间 → 允许
	if !s.shouldWake(1, windowStart.Add(-6*time.Hour)) {
		t.Error("未设定唤醒时间时应允许执行")
	}

	// 设定 21:00 唤醒
	s.scheduleWake(1, windowStart)

	// 16:20 —— 远早于窗口，跳过
	if s.shouldWake(1, time.Date(2026, 11, 3, 16, 20, 0, 0, time.Local)) {
		t.Error("窗口开始前不应唤醒")
	}
	// 20:59:30 —— 仍差 30 秒，跳过
	if s.shouldWake(1, windowStart.Add(-30*time.Second)) {
		t.Error("窗口开始前 30 秒不应唤醒")
	}
	// 21:00:30 —— 已到窗口，允许
	if !s.shouldWake(1, windowStart.Add(30*time.Second)) {
		t.Error("进入窗口后应唤醒")
	}
	// 另一个计划不受影响
	if !s.shouldWake(2, time.Date(2026, 11, 3, 16, 20, 0, 0, time.Local)) {
		t.Error("其他计划不应被影响")
	}
}

// TestIsTransientMessage 校验待核对结果不会被当成「已完成」而占用当天机会。
//
// 回归：pending（服务端返回成功但任务仍在待办）曾被写入 last_run_at，
// 导致当天真实窗口到来时不再执行——静默漏签。
func TestIsTransientMessage(t *testing.T) {
	transient := []string{
		"查寝：服务端返回成功，但任务仍在待办列表中",
		"查寝：网络请求失败",
		"查寝：待核对",
	}
	for _, msg := range transient {
		if !isTransientMessage(msg) {
			t.Errorf("应判定为可重试：%s", msg)
		}
	}
	deterministic := []string{
		"查寝：已签到，无需重复执行",
		"查寝：已过签到时间，无法补签",
		"查寝：已完成",
	}
	for _, msg := range deterministic {
		if isTransientMessage(msg) {
			t.Errorf("不应判定为可重试：%s", msg)
		}
	}
}

// TestIsTransientFailureHumanIntervention 校验需要人工介入的失败会停止重试。
//
// 回归：需要图形验证码时曾命中 transient 关键词，导致每 10 分钟撞一次登录接口。
func TestIsTransientFailureHumanIntervention(t *testing.T) {
	needsHuman := []error{
		errors.New("自动登录失败：本次登录需要图形验证码，请到账号页手动登录一次"),
		errors.New("该账号未保存密码"),
		errors.New("账号密码解析失败，请重新填写密码"),
		errors.New("账号已被冻结，请联系学校管理员"),
	}
	for _, err := range needsHuman {
		if isTransientFailure(err) {
			t.Errorf("需要人工介入的失败不应重试：%v", err)
		}
	}
	if !isTransientFailure(errors.New("网络请求失败")) {
		t.Error("网络错误应允许重试")
	}
}

// TestScheduleWakeKeepsEarliest 校验唤醒时刻取最早值。
//
// 回归：wakeAt 只覆盖不取小，后续同步发现更早窗口时会被旧时刻挡住。
func TestScheduleWakeKeepsEarliest(t *testing.T) {
	s := &Scheduler{wakeAt: map[int64]time.Time{}}
	late := time.Date(2026, 11, 3, 22, 0, 0, 0, time.Local)
	early := time.Date(2026, 11, 3, 17, 0, 0, 0, time.Local)

	s.scheduleWake(1, late)
	s.scheduleWake(1, early) // 更早的窗口应生效
	if !s.shouldWake(1, early.Add(time.Second)) {
		t.Error("应采纳更早的唤醒时刻")
	}

	// 再设一个更晚的，不应覆盖
	s.scheduleWake(1, late.Add(time.Hour))
	if s.shouldWake(1, late.Add(2*time.Hour)) == false {
		t.Error("更晚的唤醒时刻不应延后已有计划")
	}

	// 清除后恢复放行
	s.clearWake(1)
	if !s.shouldWake(1, early) {
		t.Error("清除唤醒后应允许执行")
	}
}

// 指定日期优先：选了具体日期后只在这些天执行，星期设置失效。
func TestShouldRunExplicitDates(t *testing.T) {
	// 2026-11-13 是周五 -> 星期索引 4
	friday := time.Date(2026, 11, 13, 21, 0, 0, 0, time.Local)

	plan := store.Plan{Enabled: true, Mode: "school", Days: []int{0}, Dates: []string{"2026-11-13"}}
	if !shouldRun(plan, friday) {
		t.Error("日期命中时应执行（即使星期不匹配）")
	}

	plan.Dates = []string{"2026-11-14"}
	if shouldRun(plan, friday) {
		t.Error("日期未命中时不应执行，且不能回退到星期判断")
	}

	// 清空日期后回到星期语义
	plan.Dates = nil
	plan.Days = []int{4}
	if !shouldRun(plan, friday) {
		t.Error("无日期时应按星期执行")
	}

	// 当天已执行过不重复
	plan.LastRunAt = "2026-11-13 21:00:00"
	if shouldRun(plan, friday) {
		t.Error("当天已执行过不应重复")
	}
}
