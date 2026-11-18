// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Plan 是自动签到/查寝计划。
type Plan struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId"`
	AccountID   int64  `json:"accountId"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Mode        string `json:"mode"` // school = 跟随学校下发时间；custom = 自定义时段
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`
	Days        []int  `json:"days"`
	// Dates 是指定日期（"2006-01-02"）；非空时只在这些天执行，星期设置失效。
	Dates        []string `json:"dates"`
	Enabled      bool     `json:"enabled"`
	LastRunAt    string   `json:"lastRunAt"`
	LastResult   string   `json:"lastResult"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
	AccountAlias string   `json:"accountAlias,omitempty"`
	SchoolName   string   `json:"schoolName,omitempty"`
	Username     string   `json:"accountUsername,omitempty"`
}

const planColumns = `p.id, p.user_id, p.account_id, p.name, p.kind, p.mode, p.window_start, p.window_end,
	p.days, p.dates, p.enabled, p.last_run_at, p.last_result, p.created_at, p.updated_at`

func scanPlan(scanner interface{ Scan(...any) error }) (*Plan, error) {
	var p Plan
	var days, dates string
	var enabled int
	err := scanner.Scan(&p.ID, &p.UserID, &p.AccountID, &p.Name, &p.Kind, &p.Mode, &p.WindowStart,
		&p.WindowEnd, &days, &dates, &enabled, &p.LastRunAt, &p.LastResult, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(days), &p.Days)
	if p.Days == nil {
		p.Days = []int{0, 1, 2, 3, 4, 5, 6}
	}
	_ = json.Unmarshal([]byte(dates), &p.Dates)
	return &p, nil
}

// encodeDates 序列化指定日期数组；空数组保持 "[]"（表示按星期）。
func encodeDates(dates []string) string {
	if dates == nil {
		dates = []string{}
	}
	buf, _ := json.Marshal(dates)
	return string(buf)
}

func encodeDays(days []int) string {
	if len(days) == 0 {
		days = []int{0, 1, 2, 3, 4, 5, 6}
	}
	buf, _ := json.Marshal(days)
	return string(buf)
}

// CreatePlan 新增计划。
func (db *DB) CreatePlan(ctx context.Context, p *Plan) (*Plan, error) {
	ts := now()
	p.CreatedAt, p.UpdatedAt = ts, ts
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO plans (user_id, account_id, name, kind, mode, window_start, window_end, days, dates, enabled,
			last_run_at, last_result, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,'','',?,?)`,
		p.UserID, p.AccountID, p.Name, p.Kind, p.Mode, p.WindowStart, p.WindowEnd,
		encodeDays(p.Days), encodeDates(p.Dates), enabled, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.ID, _ = res.LastInsertId()
	return p, nil
}

// UpdatePlan 更新计划。
func (db *DB) UpdatePlan(ctx context.Context, p *Plan) error {
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	p.UpdatedAt = now()
	res, err := db.ExecContext(ctx, `
		UPDATE plans SET account_id = ?, name = ?, kind = ?, mode = ?, window_start = ?, window_end = ?,
			days = ?, dates = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		p.AccountID, p.Name, p.Kind, p.Mode, p.WindowStart, p.WindowEnd, encodeDays(p.Days),
		encodeDates(p.Dates), enabled, p.UpdatedAt, p.ID, p.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound // 计划不存在或不属于该用户，不能静默返回成功
	}
	return nil
}

// TogglePlan 启停计划。
func (db *DB) TogglePlan(ctx context.Context, userID, id int64, enabled bool) error {
	flag := 0
	if enabled {
		flag = 1
	}
	res, err := db.ExecContext(ctx,
		`UPDATE plans SET enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		flag, now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlan 删除计划。
func (db *DB) DeletePlan(ctx context.Context, userID, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM plans WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPlans 返回用户的计划（附带账号信息）。
func (db *DB) ListPlans(ctx context.Context, userID int64) ([]Plan, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+planColumns+`, COALESCE(a.alias,''), COALESCE(a.school_name,''), COALESCE(a.username,'')
		FROM plans p JOIN accounts a ON a.id = p.account_id
		WHERE p.user_id = ? ORDER BY p.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		var days, dates string
		var enabled int
		if err := rows.Scan(&p.ID, &p.UserID, &p.AccountID, &p.Name, &p.Kind, &p.Mode, &p.WindowStart,
			&p.WindowEnd, &days, &dates, &enabled, &p.LastRunAt, &p.LastResult, &p.CreatedAt, &p.UpdatedAt,
			&p.AccountAlias, &p.SchoolName, &p.Username); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		_ = json.Unmarshal([]byte(days), &p.Days)
		if p.Days == nil {
			p.Days = []int{0, 1, 2, 3, 4, 5, 6}
		}
		_ = json.Unmarshal([]byte(dates), &p.Dates)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListEnabledPlans 返回全部启用中的计划（调度器使用）。
func (db *DB) ListEnabledPlans(ctx context.Context) ([]Plan, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+planColumns+` FROM plans p WHERE p.enabled = 1 ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// RecordPlanRun 记录计划最近一次执行结果。
func (db *DB) RecordPlanRun(ctx context.Context, id int64, result string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE plans SET last_run_at = ?, last_result = ?, updated_at = ? WHERE id = ?`,
		now(), result, now(), id)
	return err
}

// ResetPlanRun 清除计划当天已执行的状态，让它重新参与当天的调度。
//
// 用于失败记录把 last_run_at 占掉、或用户调整后希望当天重新尝试的场景。
func (db *DB) ResetPlanRun(ctx context.Context, userID, id int64) error {
	res, err := db.ExecContext(ctx,
		`UPDATE plans SET last_run_at = '', last_result = '', updated_at = ?
		 WHERE id = ? AND user_id = ?`, now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PlanByID 按 ID 查询单个计划（含所属账号信息）。
func (db *DB) PlanByID(ctx context.Context, userID, id int64) (*Plan, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+planColumns+`, COALESCE(a.alias,''), COALESCE(a.school_name,''), COALESCE(a.username,'')
		FROM plans p JOIN accounts a ON a.id = p.account_id
		WHERE p.id = ? AND p.user_id = ?`, id, userID)
	var p Plan
	var days, dates string
	var enabled int
	err := row.Scan(&p.ID, &p.UserID, &p.AccountID, &p.Name, &p.Kind, &p.Mode, &p.WindowStart,
		&p.WindowEnd, &days, &dates, &enabled, &p.LastRunAt, &p.LastResult, &p.CreatedAt, &p.UpdatedAt,
		&p.AccountAlias, &p.SchoolName, &p.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(days), &p.Days)
	return &p, nil
}
