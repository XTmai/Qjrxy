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
		INSERT INTO logs (user_id, account_id, plan_id, task_name, kind, source, status, message, detail, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		l.UserID, l.AccountID, l.PlanID, l.TaskName, l.Kind, l.Source, l.Status, l.Message, l.Detail, l.CreatedAt)
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
	UserID  int64
	Status  string
	Scope   string // today / week / all
	Limit   int
	Offset  int
	LogID   int64
	Kind    string
}

// ListLogs 分页查询执行记录。
func (db *DB) ListLogs(ctx context.Context, f LogFilter) ([]Log, int, error) {
	where := []string{"l.user_id = ?"}
	args := []any{f.UserID}

	switch f.Scope {
	case "today":
		start := time.Now().Truncate(24 * time.Hour)
		where = append(where, "l.created_at >= ?")
		args = append(args, start.Format(time.RFC3339))
	case "week":
		now := time.Now()
		start := now.AddDate(0, 0, -7)
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
	query := `SELECT l.id, l.user_id, l.account_id, l.plan_id, l.task_name, l.kind, l.source, l.status,
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
		if err := rows.Scan(&l.ID, &l.UserID, &l.AccountID, &l.PlanID, &l.TaskName, &l.Kind, &l.Source,
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

// LogByID 查询单条记录。
func (db *DB) LogByID(ctx context.Context, userID, id int64) (*Log, error) {
	row := db.QueryRowContext(ctx, `
		SELECT l.id, l.user_id, l.account_id, l.plan_id, l.task_name, l.kind, l.source, l.status,
			l.message, l.detail, l.created_at, COALESCE(a.alias,''), COALESCE(a.school_name,''), COALESCE(a.username,'')
		FROM logs l LEFT JOIN accounts a ON a.id = l.account_id
		WHERE l.id = ? AND l.user_id = ?`, id, userID)
	var l Log
	err := row.Scan(&l.ID, &l.UserID, &l.AccountID, &l.PlanID, &l.TaskName, &l.Kind, &l.Source,
		&l.Status, &l.Message, &l.Detail, &l.CreatedAt, &l.AccountAlias, &l.SchoolName, &l.AccountUser)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}
