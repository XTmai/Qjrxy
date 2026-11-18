// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

// settings 表的读写：admin 页可改的运行时配置都落在这里
// （诊断开关、识别服务参数等）。表结构极简（key/value），
// 值由调用方负责序列化与加密（如 API key 先过 secure.Encrypt）。
package store

import (
	"context"
	"database/sql"
	"errors"
)

// GetSetting 读取单个配置项；不存在时返回 ("", nil)。
// 其余错误原样返回：把 DB 故障当成「没配过」会让照片池上限这类
// 管理员配置静默回落到默认值。
func (db *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		// 没配过不算错误，调用方按默认值处理
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSetting 写入（upsert）单个配置项。
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// AllSettings 返回全部配置项（admin 页初始化用）。
func (db *DB) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
