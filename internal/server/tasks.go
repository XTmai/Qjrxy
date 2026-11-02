package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"qingjin/internal/service"
	"qingjin/internal/store"
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
	if !s.consumeQuota(w, r, user) {
		return
	}
	outcome, err := s.svc.RunTask(r.Context(), service.RunOptions{
		UserID:    user.ID,
		AccountID: req.AccountID,
		TaskID:    req.TaskID,
		Kind:      req.Kind,
		Source:    "manual",
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, outcome)
}

// ---------- 计划 ----------

type planRequest struct {
	AccountID   int64  `json:"accountId"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Mode        string `json:"mode"`
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`
	Days        []int  `json:"days"`
	Enabled     *bool  `json:"enabled"`
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
	writeData(w, http.StatusOK, map[string]any{"enabled": req.Enabled})
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
		enabled = true
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
		Enabled:     enabled,
	}
	return plan, nil
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

	views := make([]accountView, 0, len(accounts))
	pending := 0
	for i := range accounts {
		views = append(views, toAccountView(&accounts[i]))
	}
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
		"stats":       stats,
		"accounts":    views,
		"tasks":       tasks,
		"pendingTask": pending,
		"plans":       len(plans),
		"activePlans": activePlans,
		"recentLogs":  logs,
		"user":        user,
		"today":       service.TodayLabel(time.Now()),
	})
}

// consumeQuota 扣减额度；额度不足时写出错误并返回 false。
func (s *Server) consumeQuota(w http.ResponseWriter, r *http.Request, user *store.User) bool {
	ok, err := s.svc.DB().ConsumeQuota(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return false
	}
	if !ok {
		writeError(w, &APIError{Status: 402, Code: "quota_exhausted", Message: "额度已用完，请兑换卡密后重试"})
		return false
	}
	return true
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
