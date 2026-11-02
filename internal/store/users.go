package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("记录不存在")

// User 是平台用户。
type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Nickname     string `json:"nickname"`
	Role         string `json:"role"`
	Quota        int    `json:"quota"`
	QuotaUsed    int    `json:"quotaUsed"`
	Theme        string `json:"theme"`
	CreatedAt    string `json:"createdAt"`
	PasswordHash string `json:"-"`
}

// CreateUser 新建用户。
func (db *DB) CreateUser(ctx context.Context, username, nickname, passwordHash string, quota int) (*User, error) {
	username = normalize(username)
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO users (username, nickname, password_hash, role, quota, created_at)
		VALUES (?, ?, ?, 'user', ?, ?)`,
		username, nickname, passwordHash, quota, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("该用户名已被注册")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.UserByID(ctx, id)
}

// UserByID 按 ID 查询用户。
func (db *DB) UserByID(ctx context.Context, id int64) (*User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, username, nickname, password_hash, role, quota, quota_used, theme, created_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// UserByUsername 按用户名查询（登录用）。
func (db *DB) UserByUsername(ctx context.Context, username string) (*User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, username, nickname, password_hash, role, quota, quota_used, theme, created_at
		FROM users WHERE username = ?`, normalize(username))
	return scanUser(row)
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Nickname, &u.PasswordHash, &u.Role,
		&u.Quota, &u.QuotaUsed, &u.Theme, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CountUsers 返回用户总数（用于首次启动创建管理员）。
func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// UpdateUserTheme 保存主题偏好。
func (db *DB) UpdateUserTheme(ctx context.Context, userID int64, theme string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET theme = ? WHERE id = ?`, theme, userID)
	return err
}

// UpdateUserPassword 更新密码哈希。
func (db *DB) UpdateUserPassword(ctx context.Context, userID int64, hash string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, userID)
	return err
}

// ConsumeQuota 扣减一次额度，返回是否成功。
func (db *DB) ConsumeQuota(ctx context.Context, userID int64) (bool, error) {
	res, err := db.ExecContext(ctx,
		`UPDATE users SET quota_used = quota_used + 1 WHERE id = ? AND quota_used < quota`, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GrantQuota 增加额度（兑换卡密）。
func (db *DB) GrantQuota(ctx context.Context, userID int64, amount int) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET quota = quota + ? WHERE id = ?`, amount, userID)
	return err
}

// ---------- 会话 ----------

// CreateSession 写入登录会话。
func (db *DB) CreateSession(ctx context.Context, token string, userID int64, ttl time.Duration) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, userID, time.Now().Add(ttl).Format(time.RFC3339), now())
	return err
}

// SessionUser 校验会话并返回用户。
func (db *DB) SessionUser(ctx context.Context, token string) (*User, error) {
	var userID int64
	var expires string
	err := db.QueryRowContext(ctx, `SELECT user_id, expires_at FROM sessions WHERE token = ?`, token).
		Scan(&userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || time.Now().After(exp) {
		_, _ = db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
		return nil, ErrNotFound
	}
	return db.UserByID(ctx, userID)
}

// DeleteSession 注销会话。
func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// PurgeExpiredSessions 清理过期会话。
func (db *DB) PurgeExpiredSessions(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now())
	return err
}
