package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"qjrxy/internal/events"
	"qjrxy/internal/service"
	"qjrxy/internal/store"
)

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	tasks, err := s.svc.DB().ListTasks(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	// 附带账号别名，便于前端直接展示
	accounts, _ := s.svc.DB().ListAccounts(r.Context(), user.ID)
	alias := map[int64]string{}
	for _, a := range accounts {
		alias[a.ID] = a.Alias
	}
	out := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, map[string]any{
			"id":              t.ID,
			"accountId":       t.AccountID,
			"accountAlias":    alias[t.AccountID],
			"kind":            t.Kind,
			"kindLabel":       storeKindLabel(t.Kind),
			"signInstanceWid": t.SignInstanceWID,
			"signWid":         t.SignWID,
			"taskName":        t.TaskName,
			"sender":          t.Sender,
			"startTime":       t.StartTime,
			"endTime":         t.EndTime,
			"status":          t.Status,
			"stuSignWid":      t.StuSignWID,
			"signDate":        t.SignDate,
			"beginClock":      t.BeginClock,
			"endClock":        t.EndClock,
			"signStatus":      t.SignStatus,
			"signStatusText":  t.SignStatusText,
			"fetchedAt":       t.FetchedAt,
		})
	}
	writeData(w, http.StatusOK, map[string]any{"tasks": out})
}

func (s *Server) handleSyncTasks(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		AccountID int64 `json:"accountId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.AccountID <= 0 {
		writeError(w, badRequest("请选择账号"))
		return
	}
	tasks, err := s.svc.SyncTasks(r.Context(), user.ID, req.AccountID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"tasks": tasks, "count": len(tasks)})
}

func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		AccountID int64  `json:"accountId"`
		TaskID    int64  `json:"taskId"`
		Kind      string `json:"kind"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.AccountID <= 0 {
		writeError(w, badRequest("请选择账号"))
		return
	}
	// 额度关闭（个人自部署默认）：不预检、不扣减，直接放行。
	// 托管运营场景（QINGJIN_QUOTA_ENABLED=true）才启用额度体系。
	if s.svc.Config().QuotaEnabled {
		// 先预检额度：额度不足时直接拦下，不发起任何提交。
		if !s.hasQuota(w, r, user) {
			return
		}
	}
	// 与请求解耦：签到可能停下来等用户点验证码（最长 100 秒），
	// 期间用户刷新页面、切后台都会断开这个请求。若沿用 r.Context()，
	// 断线会让后续的 validate 与提交全部以 context canceled 失败，
	// 用户明明点完了却显示没签上。
	//
	// 预算按最坏情况留足：验证码最多重试 3 轮，每轮含一次模型识别
	// （服务端超时 60s）加上重试等待，再叠加 100s 人工等待，3 分钟会
	// 在第二轮模型超时时就被打断。6 分钟仍远小于用户可接受的等待上限。
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 6*time.Minute)
	defer cancel()
	outcome, err := s.svc.RunTask(runCtx, service.RunOptions{
		UserID:    user.ID,
		AccountID: req.AccountID,
		TaskID:    req.TaskID,
		Kind:      req.Kind,
		Source:    "manual",
	})
	if err != nil {
		// 前置拦截（未到时间、未绑设备、没有任务等）没有向学校发起请求，
		// 不应扣额度——额度在真正提交后才计费。
		writeError(w, err)
		return
	}
	// 只对「真正签到成功」扣一次额度。
	// 失败与待核对不该向用户收费——那等于当天没签上还要掉一次额度。
	// 用解耦后的 runCtx 而不是 r.Context()：客户端断开（刷新/切网）后
	// r.Context() 已取消，扣费会静默失败，成功执行却不计额度。
	if s.svc.Config().QuotaEnabled && outcome.Status == store.StatusSuccess {
		if _, qerr := s.svc.DB().ConsumeQuota(runCtx, user.ID); qerr != nil {
			log.Printf("扣减额度失败（user=%d log=%d）：%v", user.ID, outcome.LogID, qerr)
		}
	}
	writeData(w, http.StatusOK, outcome)
}

// hasQuota 预检额度是否足够；不足时写出 402 并返回 false。
func (s *Server) hasQuota(w http.ResponseWriter, r *http.Request, user *store.User) bool {
	fresh, err := s.svc.DB().UserByID(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return false
	}
	remaining := fresh.Quota - fresh.QuotaUsed
	if remaining <= 0 {
		writeError(w, &APIError{Status: 402, Code: "quota_exhausted",
			Message: "额度已用完，请兑换卡密后重试"})
		return false
	}
	return true
}

// ---------- 计划 ----------

type planRequest struct {
	AccountID   int64    `json:"accountId"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Mode        string   `json:"mode"`
	WindowStart string   `json:"windowStart"`
	WindowEnd   string   `json:"windowEnd"`
	Days        []int    `json:"days"`
	Dates       []string `json:"dates"`
	Enabled     *bool    `json:"enabled"`
}

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	plans, err := s.svc.DB().ListPlans(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"plans": plans})
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req planRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	plan, err := s.planFromRequest(r, user.ID, 0, req)
	if err != nil {
		writeError(w, err)
		return
	}
	created, err := s.svc.DB().CreatePlan(r.Context(), plan)
	if err != nil {
		writeError(w, err)
		return
	}
	s.svc.Events().Publish(user.ID, events.TopicPlans)
	writeData(w, http.StatusCreated, map[string]any{"plan": created})
}

func (s *Server) handleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req planRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	plan, err := s.planFromRequest(r, user.ID, id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.svc.DB().UpdatePlan(r.Context(), plan); err != nil {
		writeError(w, err)
		return
	}
	s.svc.Events().Publish(user.ID, events.TopicPlans)
	writeData(w, http.StatusOK, map[string]any{"plan": plan})
}

func (s *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.svc.DB().DeletePlan(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	s.svc.Events().Publish(user.ID, events.TopicPlans)
	writeData(w, http.StatusOK, map[string]any{"message": "计划已删除"})
}

func (s *Server) handleTogglePlan(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := s.svc.DB().TogglePlan(r.Context(), user.ID, id, req.Enabled); err != nil {
		writeError(w, err)
		return
	}
	s.svc.Events().Publish(user.ID, events.TopicPlans)
	writeData(w, http.StatusOK, map[string]any{"enabled": req.Enabled})
}

// handleResetPlanRun 清除计划当天已执行状态，让它当天能重新尝试。
func (s *Server) handleResetPlanRun(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.svc.DB().ResetPlanRun(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	s.svc.Events().Publish(user.ID, events.TopicPlans)
	writeData(w, http.StatusOK, map[string]any{"message": "已重置，计划将在下个周期重新检查"})
}

func (s *Server) planFromRequest(r *http.Request, userID, existingID int64, req planRequest) (*store.Plan, error) {
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "attendance"
	}
	switch kind {
	case "sign", "attendance":
	default:
		return nil, badRequest("计划仅支持签到或查寝")
	}
	if _, err := s.svc.DB().AccountByID(r.Context(), userID, req.AccountID); err != nil {
		return nil, badRequest("账号不存在")
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "school"
	}
	if mode != "school" && mode != "custom" {
		return nil, badRequest("计划模式只能是 school 或 custom")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "自动" + storeKindLabel(kind) + "计划"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	} else if existingID > 0 {
		// 编辑时未显式传 enabled：沿用数据库里的当前值，
		// 不能默认 true，否则暂停中的计划一编辑就会被重新启用。
		if current, err := s.svc.DB().PlanByID(r.Context(), userID, existingID); err == nil {
			enabled = current.Enabled
		}
	}
	dates, err := normalizeDates(req.Dates)
	if err != nil {
		return nil, err
	}
	plan := &store.Plan{
		ID:          existingID,
		UserID:      userID,
		AccountID:   req.AccountID,
		Name:        name,
		Kind:        kind,
		Mode:        mode,
		WindowStart: defaultStr(req.WindowStart, "00:10"),
		WindowEnd:   defaultStr(req.WindowEnd, "23:30"),
		Days:        normalizeDays(req.Days),
		Dates:       dates,
		Enabled:     enabled,
	}
	return plan, nil
}

// normalizeDates 校验指定日期（YYYY-MM-DD），去重排序；空数组表示按星期执行。
func normalizeDates(dates []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(dates))
	for _, raw := range dates {
		d := strings.TrimSpace(raw)
		if d == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return nil, badRequest("指定日期格式应为 YYYY-MM-DD：" + d)
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	if len(out) > 366 {
		return nil, badRequest("指定日期最多 366 天")
	}
	sort.Strings(out)
	return out, nil
}

func normalizeDays(days []int) []int {
	if len(days) == 0 {
		return []int{0, 1, 2, 3, 4, 5, 6}
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(days))
	for _, d := range days {
		if d < 0 || d > 6 || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	if len(out) == 0 {
		return []int{0, 1, 2, 3, 4, 5, 6}
	}
	// 排序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ---------- 记录 ----------

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	filter := store.LogFilter{
		UserID: user.ID,
		Status: q.Get("status"),
		Scope:  q.Get("scope"),
		Kind:   q.Get("kind"),
		Limit:  limit,
		Offset: offset,
	}
	logs, total, err := s.svc.DB().ListLogs(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"logs": logs, "total": total})
}

// handleLogTrend 返回最近 N 天的执行趋势（默认 14 天）。
func (s *Server) handleLogTrend(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 14
	}
	trend, err := s.svc.DB().LogTrend(r.Context(), user.ID, days)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"days": days, "trend": trend})
}

func (s *Server) handleGetLog(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	logEntry, err := s.svc.DB().LogByID(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"log": logEntry})
}

// ---------- 工作台 ----------

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	stats, err := s.svc.DB().TodayStats(ctx, user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	accounts, _ := s.svc.DB().ListAccounts(ctx, user.ID)
	tasks, _ := s.svc.DB().ListTasks(ctx, user.ID)
	plans, _ := s.svc.DB().ListPlans(ctx, user.ID)
	logs, _, _ := s.svc.DB().ListLogs(ctx, store.LogFilter{UserID: user.ID, Limit: 6})

	views := s.accountViews(ctx, user.ID, accounts)
	// 今日已完成的账号：按查寝成功记录去重算。
	// 不能用 account_tasks 的 status——学校签完后任务会从待办列表消失，
	// ReplaceTasks 一清空，刚签完的账号就变成「没有任务」，进度反而归零。
	doneIDs, err := s.svc.DB().TodaySuccessAccountIDs(ctx, user.ID, "attendance")
	if err != nil {
		writeError(w, err)
		return
	}
	pending := 0
	for _, t := range tasks {
		if t.Status != "done" {
			pending++
		}
	}
	activePlans := 0
	for _, p := range plans {
		if p.Enabled {
			activePlans++
		}
	}
	writeData(w, http.StatusOK, map[string]any{
		"stats":          stats,
		"accounts":       views,
		"tasks":          tasks,
		"doneAccountIDs": doneIDs,
		"pendingTask":    pending,
		"plans":          len(plans),
		"activePlans":    activePlans,
		"recentLogs":     logs,
		"user":           user,
		"today":          service.TodayLabel(time.Now()),
	})
}

func storeKindLabel(kind string) string {
	switch kind {
	case "attendance":
		return "查寝"
	case "sign":
		return "签到"
	case "collect":
		return "信息收集"
	}
	return kind
}

func defaultStr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// ---------- 审计与统计 ----------

// handleAuditLogs 返回审计用的执行记录（含流水号与耗时字段）。
func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	logs, total, err := s.svc.DB().ListLogs(r.Context(), store.LogFilter{
		UserID: user.ID,
		Status: q.Get("status"),
		Scope:  q.Get("scope"),
		Kind:   q.Get("kind"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	// 提取 detail 中的耗时/偏移字段，扁平化给前端
	type auditRow struct {
		store.Log
		LatencyMs  int64   `json:"latencyMs"`
		GeoOffsetM float64 `json:"geoOffsetM"`
		InRange    *bool   `json:"inRange"`
	}
	out := make([]auditRow, 0, len(logs))
	for _, l := range logs {
		row := auditRow{Log: l}
		var d struct {
			LatencyMs  int64   `json:"latencyMs"`
			GeoOffsetM float64 `json:"geoOffsetM"`
			InRange    *bool   `json:"inRange"`
		}
		if l.Detail != "" && json.Unmarshal([]byte(l.Detail), &d) == nil {
			row.LatencyMs = d.LatencyMs
			row.GeoOffsetM = d.GeoOffsetM
			row.InRange = d.InRange
		}
		out = append(out, row)
	}
	writeData(w, http.StatusOK, map[string]any{"logs": out, "total": total})
}

// handleStats7d 返回最近 7 天聚合指标（监控总览用）。
func (s *Server) handleStats7d(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	stats, err := s.svc.DB().AggregateLogs7d(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, stats)
}
