// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB 封装 SQLite 连接。
type DB struct {
	*sql.DB
	path string
}

// Open 打开（或创建）数据库并执行迁移。
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败：%w", err)
		}
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	raw.SetMaxOpenConns(1)
	raw.SetConnMaxLifetime(0)
	db := &DB{DB: raw, path: path}
	if err := db.migrate(); err != nil {
		raw.Close()
		return nil, err
	}
	return db, nil
}

// Path 返回数据库文件路径。
func (db *DB) Path() string { return db.path }

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	nickname      TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL DEFAULT 'user',
	theme         TEXT NOT NULL DEFAULT 'light',
	created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS accounts (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	alias          TEXT NOT NULL DEFAULT '',
	school_name    TEXT NOT NULL,
	username       TEXT NOT NULL,
	password_enc   TEXT NOT NULL DEFAULT '',
	tgc_enc        TEXT NOT NULL DEFAULT '',
	tenant_id      TEXT NOT NULL DEFAULT '',
	host           TEXT NOT NULL DEFAULT '',
	ids_url        TEXT NOT NULL DEFAULT '',
	join_type      TEXT NOT NULL DEFAULT 'CAS',
	cookies        TEXT NOT NULL DEFAULT '',
	login_status   TEXT NOT NULL DEFAULT 'unknown',
	last_login_at  TEXT NOT NULL DEFAULT '',
	last_probe_at  TEXT NOT NULL DEFAULT '',
	last_probe_msg TEXT NOT NULL DEFAULT '',
	device_id      TEXT NOT NULL DEFAULT '',
	device_model   TEXT NOT NULL DEFAULT '23127PN0CC',
	system_version TEXT NOT NULL DEFAULT '14',
	platform       TEXT NOT NULL DEFAULT 'android',
	wis_device_id  TEXT NOT NULL DEFAULT '',
	user_agent     TEXT NOT NULL DEFAULT '',
	app_version    TEXT NOT NULL DEFAULT '',
	build_id       TEXT NOT NULL DEFAULT 'VLFCNXM',
	app_user_id    TEXT NOT NULL DEFAULT '',
	address        TEXT NOT NULL DEFAULT '',
	lon            REAL NOT NULL DEFAULT 0,
	lat            REAL NOT NULL DEFAULT 0,
	abnormal       TEXT NOT NULL DEFAULT '在校',
	photo_path     TEXT NOT NULL DEFAULT '',
	photo_pool_cursor INTEGER NOT NULL DEFAULT 0,
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_user_school_user
	ON accounts(user_id, school_name, username);

CREATE TABLE IF NOT EXISTS account_photos (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	path       TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_account_photos_account ON account_photos(account_id, id);

CREATE TABLE IF NOT EXISTS account_tasks (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	account_id        INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	kind              TEXT NOT NULL,
	sign_instance_wid TEXT NOT NULL,
	sign_wid          TEXT NOT NULL DEFAULT '',
	task_name         TEXT NOT NULL DEFAULT '',
	sender            TEXT NOT NULL DEFAULT '',
	start_time        TEXT NOT NULL DEFAULT '',
	end_time          TEXT NOT NULL DEFAULT '',
	status            TEXT NOT NULL DEFAULT 'pending',
	stu_sign_wid      TEXT NOT NULL DEFAULT '',
	sign_date         TEXT NOT NULL DEFAULT '',
	begin_clock       TEXT NOT NULL DEFAULT '',
	end_clock         TEXT NOT NULL DEFAULT '',
	sign_status       TEXT NOT NULL DEFAULT '',
	sign_status_text  TEXT NOT NULL DEFAULT '',
	fetched_at        TEXT NOT NULL,
	UNIQUE(account_id, kind, sign_instance_wid)
);

CREATE TABLE IF NOT EXISTS plans (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	name          TEXT NOT NULL DEFAULT '',
	kind          TEXT NOT NULL,
	mode          TEXT NOT NULL DEFAULT 'school',
	window_start  TEXT NOT NULL DEFAULT '00:10',
	window_end    TEXT NOT NULL DEFAULT '23:30',
	days          TEXT NOT NULL DEFAULT '[0,1,2,3,4,5,6]',
	dates         TEXT NOT NULL DEFAULT '[]',
	enabled       INTEGER NOT NULL DEFAULT 1,
	last_run_at   TEXT NOT NULL DEFAULT '',
	last_result   TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS logs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	plan_id     INTEGER NOT NULL DEFAULT 0,
	trace_id    TEXT NOT NULL DEFAULT '',
	task_name   TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL DEFAULT '',
	source      TEXT NOT NULL DEFAULT 'manual',
	status      TEXT NOT NULL DEFAULT 'executing',
	message     TEXT NOT NULL DEFAULT '',
	detail      TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_logs_user_time ON logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_logs_account_time ON logs(account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

func (db *DB) migrate() error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("初始化数据库失败：%w", err)
	}
	// 增量迁移：老库补列（失败说明已存在，忽略）
	for _, stmt := range []string{
		`ALTER TABLE accounts ADD COLUMN tgc_enc TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE plans ADD COLUMN dates TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE accounts ADD COLUMN platform TEXT NOT NULL DEFAULT 'android'`,
		`ALTER TABLE accounts ADD COLUMN wis_device_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE account_tasks ADD COLUMN stu_sign_wid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE account_tasks ADD COLUMN sign_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE account_tasks ADD COLUMN begin_clock TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE account_tasks ADD COLUMN end_clock TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE account_tasks ADD COLUMN sign_status TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE account_tasks ADD COLUMN sign_status_text TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN user_agent TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN app_version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN last_probe_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN last_probe_msg TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN photo_pool_cursor INTEGER NOT NULL DEFAULT 0`,
		// app_user_id 是今日校园 App 账号的 userId，扫码取票时落库，用于防止绑错人。
		`ALTER TABLE accounts ADD COLUMN app_user_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE logs ADD COLUMN trace_id TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.Exec(stmt)
	}
	return nil
}

// now 返回统一的 RFC3339 时间字符串。
func now() string { return time.Now().Format(time.RFC3339) }

// normalize 去掉首尾空白。
func normalize(s string) string { return strings.TrimSpace(s) }
