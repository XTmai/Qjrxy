// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Task 是缓存下来的待办任务。
type Task struct {
	ID              int64  `json:"id"`
	AccountID       int64  `json:"accountId"`
	Kind            string `json:"kind"`
	SignInstanceWID string `json:"signInstanceWid"`
	SignWID         string `json:"signWid"`
	TaskName        string `json:"taskName"`
	Sender          string `json:"sender"`
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime"`
	Status          string `json:"status"`
	StuSignWID      string `json:"stuSignWid"`
	SignDate        string `json:"signDate"`
	BeginClock      string `json:"beginClock"`
	EndClock        string `json:"endClock"`
	SignStatus      string `json:"signStatus"`
	SignStatusText  string `json:"signStatusText"`
	FetchedAt       string `json:"fetchedAt"`
}

// ReplaceTasks 用最新拉取结果覆盖账号的待办缓存。
func (db *DB) ReplaceTasks(ctx context.Context, accountID int64, tasks []Task) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_tasks WHERE account_id = ?`, accountID); err != nil {
		return err
	}
	ts := now()
	for _, t := range tasks {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO account_tasks (account_id, kind, sign_instance_wid, sign_wid, task_name, sender,
				start_time, end_time, status, stu_sign_wid, sign_date, begin_clock, end_clock,
				sign_status, sign_status_text, fetched_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			accountID, t.Kind, t.SignInstanceWID, t.SignWID, t.TaskName, t.Sender,
			t.StartTime, t.EndTime, t.Status, t.StuSignWID, t.SignDate, t.BeginClock, t.EndClock,
			t.SignStatus, t.SignStatusText, ts)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				continue
			}
			return err
		}
	}
	return tx.Commit()
}

// ListTasks 返回用户的全部缓存任务。
func (db *DB) ListTasks(ctx context.Context, userID int64) ([]Task, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id, t.account_id, t.kind, t.sign_instance_wid, t.sign_wid, t.task_name, t.sender,
			t.start_time, t.end_time, t.status, t.stu_sign_wid, t.sign_date, t.begin_clock,
			t.end_clock, t.sign_status, t.sign_status_text, t.fetched_at
		FROM account_tasks t
		JOIN accounts a ON a.id = t.account_id
		WHERE a.user_id = ?
		ORDER BY t.fetched_at DESC, t.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Kind, &t.SignInstanceWID, &t.SignWID, &t.TaskName,
			&t.Sender, &t.StartTime, &t.EndTime, &t.Status, &t.StuSignWID, &t.SignDate,
			&t.BeginClock, &t.EndClock, &t.SignStatus, &t.SignStatusText, &t.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TasksByAccount 返回某账号的缓存任务。
func (db *DB) TasksByAccount(ctx context.Context, accountID int64) ([]Task, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, account_id, kind, sign_instance_wid, sign_wid, task_name, sender,
			start_time, end_time, status, stu_sign_wid, sign_date, begin_clock, end_clock,
			sign_status, sign_status_text, fetched_at
		FROM account_tasks WHERE account_id = ? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Kind, &t.SignInstanceWID, &t.SignWID, &t.TaskName,
			&t.Sender, &t.StartTime, &t.EndTime, &t.Status, &t.StuSignWID, &t.SignDate,
			&t.BeginClock, &t.EndClock, &t.SignStatus, &t.SignStatusText, &t.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TaskByID 按 ID 查询缓存任务。
func (db *DB) TaskByID(ctx context.Context, userID, id int64) (*Task, error) {
	row := db.QueryRowContext(ctx, `
		SELECT t.id, t.account_id, t.kind, t.sign_instance_wid, t.sign_wid, t.task_name, t.sender,
			t.start_time, t.end_time, t.status, t.stu_sign_wid, t.sign_date, t.begin_clock,
			t.end_clock, t.sign_status, t.sign_status_text, t.fetched_at
		FROM account_tasks t JOIN accounts a ON a.id = t.account_id
		WHERE t.id = ? AND a.user_id = ?`, id, userID)
	var t Task
	err := row.Scan(&t.ID, &t.AccountID, &t.Kind, &t.SignInstanceWID, &t.SignWID, &t.TaskName,
		&t.Sender, &t.StartTime, &t.EndTime, &t.Status, &t.StuSignWID, &t.SignDate,
		&t.BeginClock, &t.EndClock, &t.SignStatus, &t.SignStatusText, &t.FetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MarkTaskDone 标记任务已处理。
func (db *DB) MarkTaskDone(ctx context.Context, accountID int64, kind, signInstanceWID string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE account_tasks SET status = 'done'
		WHERE account_id = ? AND kind = ? AND sign_instance_wid = ?`, accountID, kind, signInstanceWID)
	return err
}
