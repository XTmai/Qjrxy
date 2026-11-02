package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"qingjin/internal/service"
	"qingjin/internal/store"
)

// Scheduler 是服务端的定时执行器。
//
// 每 30 秒扫描一次启用中的计划：命中"星期 + 时段"且当天尚未成功执行过，
// 就挑一个待办任务提交。计划表里的 last_run_at 用于去重，避免重复打卡。
type Scheduler struct {
	svc      *service.Service
	db       *store.DB
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  sync.Mutex
}

// New 创建调度器。
func New(svc *service.Service) *Scheduler {
	return &Scheduler{
		svc:      svc,
		db:       svc.DB(),
		interval: 30 * time.Second,
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
	log.Printf("调度器已启动，扫描间隔 %s", s.interval)
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
		s.runPlan(ctx, plan, now)
	}
}

// shouldRun 判断计划此刻是否应当执行。
func shouldRun(plan store.Plan, now time.Time) bool {
	if !plan.Enabled {
		return false
	}
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

func (s *Scheduler) runPlan(ctx context.Context, plan store.Plan, now time.Time) {
	account, err := s.db.AccountByIDInternal(ctx, plan.AccountID)
	if err != nil {
		_ = s.db.RecordPlanRun(ctx, plan.ID, "账号不存在："+err.Error())
		return
	}
	kind := plan.Kind
	// 跟随学校时间时，先同步任务；若当前没有待办则等下一轮。
	if plan.Mode == "school" {
		if _, err := s.svc.SyncTasks(ctx, account.UserID, account.ID); err != nil {
			_ = s.db.RecordPlanRun(ctx, plan.ID, "同步任务失败：未能获取待办")
			log.Printf("计划 %d 同步任务失败：%v", plan.ID, err)
			// 同步失败时不再继续，等待下一轮（last_run_at 不写，避免误判已完成）
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
		_ = s.db.RecordPlanRun(ctx, plan.ID, "执行失败："+err.Error())
		log.Printf("计划 %d 执行失败：%v", plan.ID, err)
		return
	}
	result := fmt.Sprintf("%s：%s", outcome.Task, outcome.Message)
	switch outcome.Status {
	case store.StatusSuccess:
		_ = s.db.RecordPlanRun(ctx, plan.ID, result)
	default:
		// 失败也记录时间，避免同一时段内反复重试打爆接口
		_ = s.db.RecordPlanRun(ctx, plan.ID, result)
	}
	log.Printf("计划 %d 执行完成：%s", plan.ID, result)
}
