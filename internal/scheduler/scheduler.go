package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"qjrxy/internal/events"
	"qjrxy/internal/service"
	"qjrxy/internal/store"
)

// Scheduler 是服务端的定时执行器，包含两条循环：
//
//  1. 计划执行（每 30 秒）：命中"星期 + 时段"且当天尚未成功执行的计划，挑一个可签到的任务提交。
//  2. 登录态探测（按需）：在查寝可能下发/可签到的时段内，按账号探测会话；
//     失效时用保存的密码自动重登并同步任务，用户不必手动点「拉取任务」。
//
// 探测频率刻意保守：查寝任务通常只在傍晚到夜间下发，全天高频探测既无意义，
// 又会让学生账号呈现出非真人行为特征（风控看重请求频次与时间分布）。
type Scheduler struct {
	svc      *service.Service
	db       *store.DB
	interval time.Duration
	// probeCheckEvery 是「多久检查一次是否该探测」的粒度。
	probeCheckEvery time.Duration
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	running         sync.Mutex
	probeRunning    sync.Mutex
	lastProbeAt     time.Time
	// attemptMu 保护 attemptAt / wakeAt：记录每个计划上次尝试时间与下次唤醒时间，
	// 避免任务还没到窗口时每 30 秒重复同步任务、重复提交。
	attemptMu sync.Mutex
	attemptAt map[int64]time.Time
	wakeAt    map[int64]time.Time
}

// attemptGap 是同一计划两次「尝试」之间的最小间隔。
//
// 没有它的话：任务未到签到时间 → 不写 last_run_at → 30 秒后再次触发
// → 每分钟 2 次任务同步，一天数千次请求，既浪费也容易被判为异常。
const attemptGap = 10 * time.Minute

// New 创建调度器。
func New(svc *service.Service) *Scheduler {
	return &Scheduler{
		svc:             svc,
		db:              svc.DB(),
		interval:        30 * time.Second,
		probeCheckEvery: time.Minute,
		attemptAt:       map[int64]time.Time{},
		wakeAt:          map[int64]time.Time{},
	}
}

// Start 在后台启动调度循环。
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		// 启动后先等一轮，避免与初始化并发
		time.Sleep(3 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()

	// 登录态探测循环：每分钟看一眼是否到了该探测的时间点，
	// 真正的探测由 shouldProbeNow 按「时段 + 间隔」决定，不会每分钟都发请求。
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.probeCheckEvery)
		defer ticker.Stop()
		// 启动后稍等，避免和初始化、用户刚打开页面的操作撞在一起
		time.Sleep(20 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				// 顺手回收过期的登录中间态：里面有明文密码，不能长期留在内存里。
				s.svc.PruneExpiredStates()
				if !shouldProbeNow(now, s.lastProbeAt) {
					continue
				}
				s.probeTick(ctx, now)
			}
		}
	}()
	log.Printf("调度器已启动，计划扫描 %s，登录态探测仅在活跃时段按需执行", s.interval)
}

// probeWindows 是允许探测的时段（本地时间，按小时计）。
//
// 依据：查寝任务通常在傍晚到夜间下发、当天内完成。凌晨到清晨探测没有收益，
// 且这段时间的异常请求更容易被识别为非真人行为。
var probeWindows = [][2]int{
	{7, 9},   // 早间：白天签到类任务开始下发
	{11, 13}, // 午间
	{16, 23}, // 傍晚到夜间：查寝主时段
}

// probeGap 是同一账号两次探测之间的最小间隔。
//
// 按最活跃时段（16:00-23:00 共 7 小时）算，每小时探测 1 次 ≈ 每天 15 次左右，
// 与真人一天打开 App 的频次相当，不会形成明显的机器特征。
const probeGap = time.Hour

// shouldProbeNow 判断此刻是否应该发起一轮探测。
func shouldProbeNow(now, last time.Time) bool {
	if !inProbeWindow(now) {
		return false
	}
	// 同一天内按 probeGap 节流；跨天或首次探测直接放行
	if !last.IsZero() && now.Sub(last) < probeGap {
		return false
	}
	return true
}

// inProbeWindow 判断当前小时是否落在允许探测的时段内。
func inProbeWindow(t time.Time) bool {
	h := t.Hour()
	for _, w := range probeWindows {
		if h >= w[0] && h < w[1] {
			return true
		}
	}
	return false
}

// probeTick 探测所有账号的登录态并同步任务。
func (s *Scheduler) probeTick(ctx context.Context, now time.Time) {
	if !s.probeRunning.TryLock() {
		return
	}
	defer s.probeRunning.Unlock()
	s.lastProbeAt = now

	results := s.svc.ProbeAllAccounts(ctx)
	if len(results) == 0 {
		return
	}
	var ok, relogged, expired int
	for _, r := range results {
		switch r.Status {
		case "authorized":
			ok++
		case "relogged":
			relogged++
			log.Printf("账号 %s 会话已自动恢复", r.AccountName)
		default:
			expired++
			log.Printf("账号 %s 需要重新登录：%s", r.AccountName, r.Message)
		}
	}
	log.Printf("登录态探测完成：正常 %d，自动恢复 %d，需处理 %d", ok, relogged, expired)
}

// Stop 停止调度循环并等待当前轮结束。
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Scheduler) tick(ctx context.Context) {
	// 同一时刻只跑一轮，避免重入。
	if !s.running.TryLock() {
		return
	}
	defer s.running.Unlock()

	plans, err := s.db.ListEnabledPlans(ctx)
	if err != nil {
		log.Printf("读取计划失败：%v", err)
		return
	}
	now := time.Now()
	for _, plan := range plans {
		if !shouldRun(plan, now) {
			continue
		}
		// 窗口驱动：任务未到签到时间时已记录唤醒时刻，到点前一律跳过。
		if !s.shouldWake(plan.ID, now) {
			continue
		}
		s.runPlan(ctx, plan, now)
	}
}

// shouldRun 判断计划此刻是否应当执行。
//
// 两道门槛缺一不可：
//  1. 计划层面：星期匹配、当天未执行过、自定义时段命中。
//  2. 任务层面：账号下确实存在「此刻可以提交」的查寝任务。
//
// 只看计划层面会让「跟随学校时间」的计划在创建后立刻触发一次无效提交：
// 任务当天还没到签到时间，服务端回「查寝未开始」，既浪费请求又占掉当天机会。
func shouldRun(plan store.Plan, now time.Time) bool {
	if !plan.Enabled {
		return false
	}
	// 指定日期优先：一旦选了具体日期，只在那些天执行，星期设置失效。
	if len(plan.Dates) > 0 {
		today := now.Format("2006-01-02")
		matched := false
		for _, d := range plan.Dates {
			if d == today {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	} else {
		// 星期过滤（0 = 周一）
		day := (int(now.Weekday()) + 6) % 7
		matched := false
		for _, d := range plan.Days {
			if d == day {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	// 当天已执行过就不再重复
	if strings.HasPrefix(plan.LastRunAt, now.Format("2006-01-02")) {
		return false
	}
	if plan.Mode == "custom" {
		if !inWindow(now, plan.WindowStart, plan.WindowEnd) {
			return false
		}
	}
	return true
}

// inWindow 判断当前时间是否落在 HH:MM - HH:MM 区间内（支持跨天）。
func inWindow(now time.Time, start, end string) bool {
	startMin, ok1 := parseClock(start)
	endMin, ok2 := parseClock(end)
	if !ok1 || !ok2 {
		return true
	}
	current := now.Hour()*60 + now.Minute()
	if startMin <= endMin {
		return current >= startMin && current <= endMin
	}
	return current >= startMin || current <= endMin
}

func parseClock(v string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// scheduleWake 记录该计划的下次执行时刻（任务窗口开始时间）。
//
// 取较早值：缓存里的任务窗口可能在后续同步中变化（旧任务消失、新任务更早开始），
// 若只覆盖不取小，就会因旧时刻太晚而错过更早的真实窗口。
func (s *Scheduler) scheduleWake(planID int64, start time.Time) {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	if s.wakeAt == nil {
		s.wakeAt = map[int64]time.Time{}
	}
	if existing, ok := s.wakeAt[planID]; ok && existing.Before(start) {
		return // 已有更早的唤醒时刻，保持不变
	}
	s.wakeAt[planID] = start
}

// clearWake 清除唤醒时刻（执行后或计划被编辑/重置时调用）。
func (s *Scheduler) clearWake(planID int64) {
	s.attemptMu.Lock()
	delete(s.wakeAt, planID)
	s.attemptMu.Unlock()
}

// shouldWake 判断计划此刻是否已到唤醒时刻（未设定时视为已到）。
func (s *Scheduler) shouldWake(planID int64, now time.Time) bool {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	wake, ok := s.wakeAt[planID]
	if !ok {
		return true
	}
	return !now.Before(wake)
}

// claimAttempt 判断该计划本次是否允许尝试执行，并记录尝试时间。
//
// 这是「未到窗口时不消耗当天机会」与「不重复轰炸接口」之间的折中：
// 允许周期性重试，但两次之间至少间隔 attemptGap。
func (s *Scheduler) claimAttempt(planID int64, now time.Time) bool {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	if s.attemptAt == nil {
		s.attemptAt = map[int64]time.Time{}
	}
	if last, ok := s.attemptAt[planID]; ok && now.Sub(last) < attemptGap {
		return false
	}
	s.attemptAt[planID] = now
	return true
}

// releaseAttempt 撤销一次「尝试」记账。
//
// 只用于「判断了窗口但并没有真正提交」的路径。等窗口不是尝试：若把它
// 算作尝试，attemptGap 的配额就浪费在本地判断上——调度器重启/整点探测
// 恰好在窗口开始前几分钟扫过一次时，窗口真正开始的那一刻会被自己的
// 节流挡回去（2026-11-14 实测：20:57 重启，21:00 的窗口全部被吃掉）。
func (s *Scheduler) releaseAttempt(planID int64) {
	s.attemptMu.Lock()
	delete(s.attemptAt, planID)
	s.attemptMu.Unlock()
}

func (s *Scheduler) runPlan(ctx context.Context, plan store.Plan, now time.Time) {
	// 节流闸门：同一计划 10 分钟内只尝试一次。
	// 任务未到窗口时不能写 last_run_at（会丢掉当天机会），若不节流就会
	// 每 30 秒重复同步任务再提交，一天产生数千次无效请求。
	if !s.claimAttempt(plan.ID, now) {
		return
	}
	// 本轮已到唤醒时刻，清除它：后续由 shouldRun 的「当天是否已执行」把关，
	// 避免第二天的调度被昨天的唤醒时刻挡住。
	s.clearWake(plan.ID)

	account, err := s.db.AccountByIDInternal(ctx, plan.AccountID)
	if err != nil {
		s.recordPlanRun(ctx, plan, "账号不存在："+err.Error())
		return
	}
	kind := plan.Kind
	// 窗口驱动：先用本地缓存算出任务窗口，决定是「现在执行」还是「等到点」。
	//
	// 同步过一次任务后，窗口信息就在本地了。此后不必反复请求接口试探，
	// 直接按 beginClock 算出唤醒时刻即可——这正是「已知窗口 → 窗口内尽快执行」。
	if w, ok := s.svc.NextTaskWindow(ctx, account.ID, kind); ok {
		if !w.Ready {
			s.scheduleWake(plan.ID, w.Start)
			// 只是等窗口，没有向学校发请求：撤销本次尝试记账，
			// 保证窗口开始时不被 attemptGap 挡住。
			s.releaseAttempt(plan.ID)
			log.Printf("计划 %d 等待任务窗口：%s 于 %s 开始", plan.ID, w.TaskName,
				w.Start.Format("15:04"))
			return
		}
	} else {
		// 本地没有可用窗口（可能任务还没下发）：此时才需要同步一次。
		// custom 模式同样要同步，否则任务在计划时段内才下发时会整段错过。
		if _, err := s.svc.SyncTasks(ctx, account.UserID, account.ID); err != nil {
			// 同步失败多数是临时问题（网络、会话），不能占用当天机会，
			// 否则一次抖动就会让计划当天彻底不执行。
			s.releaseAttempt(plan.ID)
			log.Printf("计划 %d 同步任务失败，稍后重试：%v", plan.ID, err)
			return
		}
		w, ok := s.svc.NextTaskWindow(ctx, account.ID, kind)
		if !ok {
			// 只读了本地缓存/同步结果，没有提交：撤销记账。
			s.releaseAttempt(plan.ID)
			log.Printf("计划 %d 当前无可用查寝任务，等待下个周期", plan.ID)
			return
		}
		if !w.Ready {
			s.scheduleWake(plan.ID, w.Start)
			s.releaseAttempt(plan.ID)
			log.Printf("计划 %d 等待任务窗口：%s 于 %s 开始", plan.ID, w.TaskName,
				w.Start.Format("15:04"))
			return
		}
	}

	outcome, err := s.svc.RunTask(ctx, service.RunOptions{
		UserID:    account.UserID,
		AccountID: account.ID,
		Kind:      kind,
		Source:    "auto",
		PlanID:    plan.ID,
	})
	if err != nil {
		// 「还没到签到时间」属于正常等待，不能算当天已执行——
		// 否则一旦在错误的时间点触发一次，当天就再也不会重试了。
		if isWaitingForWindow(err) {
			// 前置校验就拦下了，请求没发出去：同样不算一次尝试。
			s.releaseAttempt(plan.ID)
			log.Printf("计划 %d 等待签到窗口：%v", plan.ID, err)
			return
		}
		// 临时性失败（网络、会话、服务端异常）同样不写 last_run_at：
		// 保留当天的执行机会，由 attemptGap 控制重试频率。
		if isTransientFailure(err) {
			log.Printf("计划 %d 临时失败，稍后重试：%v", plan.ID, err)
			return
		}
		s.recordPlanRun(ctx, plan, "执行失败："+err.Error())
		log.Printf("计划 %d 执行失败：%v", plan.ID, err)
		return
	}
	result := fmt.Sprintf("%s：%s", outcome.Task, outcome.Message)
	if outcome.Status == store.StatusSuccess {
		s.recordPlanRun(ctx, plan, result)
		log.Printf("计划 %d 执行完成：%s", plan.ID, result)
		return
	}
	// 设备需辅导员授权（code=3「更换手机频繁」类）：确定性权限拒绝，
	// 重试注定失败，还会给学校风控累积失败提交记录。停止当天自动重试，
	// 结果里给出指引；用户授权后在计划卡片点「重置今日」或手动执行恢复。
	if service.IsDeviceAuthRequired("", outcome.Message) {
		s.recordPlanRun(ctx, plan, "需人工处理（设备需辅导员授权）："+outcome.Message)
		log.Printf("计划 %d 需人工处理：设备需授权，已停止当天自动重试", plan.ID)
		return
	}
	// 待核对与临时性失败都不写 last_run_at，保留当天机会：
	// 这类结果不代表任务已经处理完，后续窗口内仍应继续尝试。
	// 只有「已过期 / 已签到」这类确定性结果才占用当天名额。
	if outcome.Status == store.StatusPending || isTransientMessage(outcome.Message) {
		log.Printf("计划 %d 本次未完成，保留当天机会：%s", plan.ID, result)
		return
	}
	s.recordPlanRun(ctx, plan, result)
	log.Printf("计划 %d 执行结束（不再重试）：%s", plan.ID, result)
}

// recordPlanRun 写入计划执行结果并通知在线页面。
// 计划卡片的「上次执行/上次结果」来自 plans 表，不在 RunTask 的日志事件里，
// 单独广播 plans 主题，首页与日程页才能同步变化。
func (s *Scheduler) recordPlanRun(ctx context.Context, plan store.Plan, result string) {
	_ = s.db.RecordPlanRun(ctx, plan.ID, result)
	s.svc.Events().Publish(plan.UserID, events.TopicPlans)
}

// isTransientMessage 判断执行结果的文案是否属于可重试的临时情况。
func isTransientMessage(msg string) bool {
	deterministic := []string{"已过签到时间", "已签到", "无需重复", "已完成", "更换手机", "手机授权", "辅导猫", "密码错误"}
	for _, kw := range deterministic {
		if strings.Contains(msg, kw) {
			return false
		}
	}
	return true
}

// isTransientFailure 判断失败是否属于「可以稍后重试」的临时问题。
//
// 网络错误、会话失效、服务端异常都是临时的；而账号未绑设备、学校明确拒绝、
// 任务已过期这类是确定性的，重试没有意义。
func isTransientFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// 确定性失败：重试也不会成功
	deterministic := []string{
		"已过签到时间", "已签到", "无需重复", "尚未绑定设备", "请先在账号里扫码",
		"额度已用完", "没有待处理的查寝任务", "今日查寝任务已完成",
		// 需要人工介入的情况：自动重试只会反复撞登录接口（风控风险），
		// 必须停下来等用户处理。
		"需要图形验证码", "手动登录", "未保存密码", "密码解析失败",
		"账号已被冻结", "账号已停用", "账号正在审核", "没有登录权限",
	}
	for _, kw := range deterministic {
		if strings.Contains(msg, kw) {
			return false
		}
	}
	// 临时性失败
	transient := []string{
		"网络", "超时", "timeout", "连接", "EOF",
		"登录状态", "重新登录", "会话", "自动登录失败",
		"500", "502", "503", "504", "服务端", "接口返回",
	}
	for _, kw := range transient {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// isWaitingForWindow 判断错误是否属于「任务还没到签到时间」。
//
// 这类情况应当保留当天的执行机会，等任务进入可签到时段后再执行。
func isWaitingForWindow(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "还没到签到时间") ||
		strings.Contains(msg, "还未到签到时间") ||
		strings.Contains(msg, "等待下一个时间窗口")
}
