package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Log 是一次执行记录。
type Log struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"userId"`
	AccountID    int64  `json:"accountId"`
	PlanID       int64  `json:"planId"`
	TraceID      string `json:"traceId"`
	TaskName     string `json:"taskName"`
	Kind         string `json:"kind"`
	Source       string `json:"source"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Detail       string `json:"detail"`
	CreatedAt    string `json:"createdAt"`
	AccountAlias string `json:"accountAlias,omitempty"`
	SchoolName   string `json:"schoolName,omitempty"`
	AccountUser  string `json:"accountUsername,omitempty"`
}

// LogStatus 取值。
const (
	StatusExecuting = "executing"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusPending   = "pending"
)

// CreateLog 写入一条执行记录，返回 ID。
func (db *DB) CreateLog(ctx context.Context, l *Log) (int64, error) {
	if l.CreatedAt == "" {
		l.CreatedAt = now()
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO logs (user_id, account_id, plan_id, trace_id, task_name, kind, source, status, message, detail, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		l.UserID, l.AccountID, l.PlanID, l.TraceID, l.TaskName, l.Kind, l.Source, l.Status, l.Message, l.Detail, l.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateLog 更新执行结果。
func (db *DB) UpdateLog(ctx context.Context, id int64, status, message, detail string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE logs SET status = ?, message = ?, detail = ? WHERE id = ?`, status, message, detail, id)
	return err
}

// LogFilter 是执行记录查询条件。
type LogFilter struct {
	UserID int64
	Status string
	Scope  string // today / week / all
	Limit  int
	Offset int
	LogID  int64
	Kind   string
}

// ListLogs 分页查询执行记录。
func (db *DB) ListLogs(ctx context.Context, f LogFilter) ([]Log, int, error) {
	where := []string{"l.user_id = ?"}
	args := []any{f.UserID}

	switch f.Scope {
	case "today":
		// 按本地日期零点截断。原先用 Truncate(24h) 以 UTC 零点为基准，
		// 东八区下等于当天 08:00，与 TodayStats 的本地日期口径不一致，
		// 会造成工作台显示 0 条、记录页却能查到。
		start := startOfToday()
		where = append(where, "l.created_at >= ?")
		args = append(args, start.Format(time.RFC3339))
	case "week":
		start := startOfToday().AddDate(0, 0, -6)
		where = append(where, "l.created_at >= ?")
		args = append(args, start.Format(time.RFC3339))
	}
	if f.Status != "" && f.Status != "all" {
		where = append(where, "l.status = ?")
		args = append(args, f.Status)
	}
	if f.Kind != "" && f.Kind != "all" {
		where = append(where, "l.kind = ?")
		args = append(args, f.Kind)
	}
	clause := ""
	for i, w := range where {
		if i == 0 {
			clause = w
		} else {
			clause += " AND " + w
		}
	}

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM logs l WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT l.id, l.user_id, l.account_id, l.plan_id, l.trace_id, l.task_name, l.kind, l.source, l.status,
		l.message, l.detail, l.created_at, COALESCE(a.alias,''), COALESCE(a.school_name,''), COALESCE(a.username,'')
		FROM logs l LEFT JOIN accounts a ON a.id = l.account_id
		WHERE ` + clause + ` ORDER BY l.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, f.Offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Log{}
	for rows.Next() {
		var l Log
		if err := rows.Scan(&l.ID, &l.UserID, &l.AccountID, &l.PlanID, &l.TraceID, &l.TaskName, &l.Kind, &l.Source,
			&l.Status, &l.Message, &l.Detail, &l.CreatedAt, &l.AccountAlias, &l.SchoolName,
			&l.AccountUser); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// LogStats 是工作台的统计数字。
type LogStats struct {
	TodayTotal   int `json:"todayTotal"`
	TodaySuccess int `json:"todaySuccess"`
	TodayFailed  int `json:"todayFailed"`
	TodayPending int `json:"todayPending"`
}

// TodayStats 统计今天的执行情况。
func (db *DB) TodayStats(ctx context.Context, userID int64) (LogStats, error) {
	start := time.Now().Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM logs
		WHERE user_id = ? AND substr(created_at, 1, 10) = ?
		GROUP BY status`, userID, start)
	if err != nil {
		return LogStats{}, err
	}
	defer rows.Close()
	var stats LogStats
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return stats, err
		}
		stats.TodayTotal += n
		switch status {
		case StatusSuccess:
			stats.TodaySuccess += n
		case StatusFailed:
			stats.TodayFailed += n
		default:
			stats.TodayPending += n
		}
	}
	return stats, rows.Err()
}

// DayStat 是单天的执行统计（趋势图用）。
type DayStat struct {
	Date    string `json:"date"` // YYYY-MM-DD
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
	Pending int    `json:"pending"`
}

// LogTrend 统计最近 days 天（按本地日期）每天的成功/失败/待核对数量，
// 缺失的日期补零，保证前端可以直接按天画图。
func (db *DB) LogTrend(ctx context.Context, userID int64, days int) ([]DayStat, error) {
	if days <= 0 || days > 90 {
		days = 14
	}
	start := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `
		SELECT substr(created_at, 1, 10) AS d, status, COUNT(*)
		FROM logs
		WHERE user_id = ? AND substr(created_at, 1, 10) >= ?
		GROUP BY d, status`, userID, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDate := map[string]map[string]int{}
	for rows.Next() {
		var date, status string
		var n int
		if err := rows.Scan(&date, &status, &n); err != nil {
			return nil, err
		}
		if byDate[date] == nil {
			byDate[date] = map[string]int{}
		}
		byDate[date][status] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	base, _ := time.Parse("2006-01-02", start)
	out := make([]DayStat, 0, days)
	for i := 0; i < days; i++ {
		date := base.AddDate(0, 0, i).Format("2006-01-02")
		day := DayStat{Date: date}
		for status, n := range byDate[date] {
			switch status {
			case StatusSuccess:
				day.Success = n
			case StatusFailed:
				day.Failed = n
			default:
				day.Pending += n
			}
		}
		out = append(out, day)
	}
	return out, nil
}

// startOfToday 返回本地时区今天的零点。
//
// 统一「今天」的口径：TodayStats、ListLogs(today/week)、LogTrend 都用它，
// 避免各处理解不同（UTC 零点 vs 本地零点）导致数字对不上。
func startOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// LogStats7d 是最近 7 天的聚合指标，供监控总览页使用。
type LogStats7d struct {
	Total       int     `json:"total"`
	Success     int     `json:"success"`
	SuccessRate float64 `json:"successRate"` // 0-100
	AvgLatency  int     `json:"avgLatencyMs"`
	// 平均地理偏移（米），从 detail.geoOffsetM 取平均
	AvgGeoOffset float64 `json:"avgGeoOffsetM"`
}

// AggregateLogs7d 统计最近 7 天的执行情况。
func (db *DB) AggregateLogs7d(ctx context.Context, userID int64) (LogStats7d, error) {
	since := startOfToday().AddDate(0, 0, -6).Format(time.RFC3339)
	rows, err := db.QueryContext(ctx, `
		SELECT status, COUNT(*), COALESCE(AVG(CAST(json_extract(detail, '$.latencyMs') AS REAL)), 0)
		FROM logs
		WHERE user_id = ? AND created_at >= ?
		GROUP BY status`, userID, since)
	if err != nil {
		return LogStats7d{}, err
	}
	defer rows.Close()

	var out LogStats7d
	var latencySum, latencyCount float64
	for rows.Next() {
		var status string
		var n int
		var avgLat float64
		if err := rows.Scan(&status, &n, &avgLat); err != nil {
			return out, err
		}
		out.Total += n
		if status == StatusSuccess {
			out.Success += n
		}
		if avgLat > 0 {
			latencySum += avgLat * float64(n)
			latencyCount += float64(n)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if out.Total > 0 {
		out.SuccessRate = float64(out.Success) / float64(out.Total) * 100
	}
	if latencyCount > 0 {
		out.AvgLatency = int(latencySum / latencyCount)
	}

	// 平均地理偏移单独查（detail.geoOffsetM）
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(CAST(json_extract(detail, '$.geoOffsetM') AS REAL)), 0)
		FROM logs
		WHERE user_id = ? AND created_at >= ?
		  AND json_extract(detail, '$.geoOffsetM') IS NOT NULL`, userID, since).Scan(&out.AvgGeoOffset)
	return out, nil
}

// LogByID 查询单条记录。
func (db *DB) LogByID(ctx context.Context, userID, id int64) (*Log, error) {
	row := db.QueryRowContext(ctx, `
		SELECT l.id, l.user_id, l.account_id, l.plan_id, l.trace_id, l.task_name, l.kind, l.source, l.status,
			l.message, l.detail, l.created_at, COALESCE(a.alias,''), COALESCE(a.school_name,''), COALESCE(a.username,'')
		FROM logs l LEFT JOIN accounts a ON a.id = l.account_id
		WHERE l.id = ? AND l.user_id = ?`, id, userID)
	var l Log
	err := row.Scan(&l.ID, &l.UserID, &l.AccountID, &l.PlanID, &l.TraceID, &l.TaskName, &l.Kind, &l.Source,
		&l.Status, &l.Message, &l.Detail, &l.CreatedAt, &l.AccountAlias, &l.SchoolName, &l.AccountUser)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}
