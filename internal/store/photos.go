package store

import (
	"context"
	"database/sql"
	"errors"
)

// AccountPhoto 是账号照片池里的一张照片。
//
// 文件路径与单张照片一样是相对 <DataDir>/photos 的斜杠路径；
// 顺序即 id 顺序（上传顺序），执行时由 accounts.photo_pool_cursor 轮换。
type AccountPhoto struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	AccountID int64  `json:"accountId"`
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
}

// PhotoPoolStats 是账号照片池的汇总，用于列表页展示数量与封面。
type PhotoPoolStats struct {
	Count   int
	FirstID int64
}

// CreateAccountPhoto 写入一张池内照片。
func (db *DB) CreateAccountPhoto(ctx context.Context, p *AccountPhoto) error {
	p.CreatedAt = now()
	res, err := db.ExecContext(ctx, `
		INSERT INTO account_photos (user_id, account_id, path, created_at)
		VALUES (?, ?, ?, ?)`,
		p.UserID, p.AccountID, p.Path, p.CreatedAt)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// ErrPoolFull 表示账号照片池已达单账号上限。
var ErrPoolFull = errors.New("照片池已满")

// CreateAccountPhotoWithinLimit 在同一个立即事务里「数一张、插一张」。
//
// 之前的实现是请求开头 COUNT 一次，然后循环逐张 INSERT：并发上传时
// 两个请求都读到旧数量，各自插入，实际张数可以突破上限。
// BEGIN IMMEDIATE 会在事务内拿到写锁，COUNT 与 INSERT 之间不会再插入别人。
func (db *DB) CreateAccountPhotoWithinLimit(ctx context.Context, p *AccountPhoto, max int) error {
	p.CreatedAt = now()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM account_photos WHERE user_id = ? AND account_id = ?`,
		p.UserID, p.AccountID).Scan(&n); err != nil {
		return err
	}
	if n >= max {
		return ErrPoolFull
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO account_photos (user_id, account_id, path, created_at)
		VALUES (?, ?, ?, ?)`,
		p.UserID, p.AccountID, p.Path, p.CreatedAt)
	if err != nil {
		return err
	}
	if p.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	return tx.Commit()
}

// ListAccountPhotos 返回账号的池内照片（上传顺序）。
func (db *DB) ListAccountPhotos(ctx context.Context, userID, accountID int64) ([]AccountPhoto, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, account_id, path, created_at
		FROM account_photos WHERE user_id = ? AND account_id = ?
		ORDER BY id`, userID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccountPhoto{}
	for rows.Next() {
		var p AccountPhoto
		if err := rows.Scan(&p.ID, &p.UserID, &p.AccountID, &p.Path, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AccountPhotoByID 查询一张池内照片（带账号/用户归属校验）。
func (db *DB) AccountPhotoByID(ctx context.Context, userID, accountID, photoID int64) (*AccountPhoto, error) {
	var p AccountPhoto
	err := db.QueryRowContext(ctx, `
		SELECT id, user_id, account_id, path, created_at
		FROM account_photos WHERE id = ? AND user_id = ? AND account_id = ?`,
		photoID, userID, accountID).
		Scan(&p.ID, &p.UserID, &p.AccountID, &p.Path, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeleteAccountPhoto 删除一条池记录并返回文件路径，供调用方删除文件。
func (db *DB) DeleteAccountPhoto(ctx context.Context, userID, accountID, photoID int64) (string, error) {
	var path string
	err := db.QueryRowContext(ctx, `
		SELECT path FROM account_photos
		WHERE id = ? AND user_id = ? AND account_id = ?`,
		photoID, userID, accountID).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	res, err := db.ExecContext(ctx, `
		DELETE FROM account_photos WHERE id = ? AND user_id = ? AND account_id = ?`,
		photoID, userID, accountID)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return path, nil
}

// CountAccountPhotos 返回账号池内照片数量。
func (db *DB) CountAccountPhotos(ctx context.Context, userID, accountID int64) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM account_photos WHERE user_id = ? AND account_id = ?`,
		userID, accountID).Scan(&n)
	return n, err
}

// PhotoPoolStatsByUser 一次查询返回该用户所有账号的池数量与首张照片 ID。
func (db *DB) PhotoPoolStatsByUser(ctx context.Context, userID int64) (map[int64]PhotoPoolStats, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT account_id, COUNT(*), MIN(id)
		FROM account_photos WHERE user_id = ?
		GROUP BY account_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]PhotoPoolStats{}
	for rows.Next() {
		var accountID int64
		var stats PhotoPoolStats
		var first sql.NullInt64
		if err := rows.Scan(&accountID, &stats.Count, &first); err != nil {
			return nil, err
		}
		stats.FirstID = first.Int64
		out[accountID] = stats
	}
	return out, rows.Err()
}

// PhotoPoolStats 返回单个账号的池汇总。
func (db *DB) PhotoPoolStats(ctx context.Context, userID, accountID int64) (PhotoPoolStats, error) {
	var stats PhotoPoolStats
	var first sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(id) FROM account_photos
		WHERE user_id = ? AND account_id = ?`, userID, accountID).
		Scan(&stats.Count, &first)
	if err != nil {
		return stats, err
	}
	stats.FirstID = first.Int64
	return stats, nil
}

// NextAccountPhoto 按上传顺序轮换取下一张池内照片。
//
// 游标存在 accounts.photo_pool_cursor，事务内读-选-更新，避免并发执行
// （手动 + 定时）时两次拿到同一张或跳号。
func (db *DB) NextAccountPhoto(ctx context.Context, userID, accountID int64) (*AccountPhoto, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var cursor int
	err = tx.QueryRowContext(ctx,
		`SELECT photo_pool_cursor FROM accounts WHERE id = ? AND user_id = ?`,
		accountID, userID).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, account_id, path, created_at
		FROM account_photos WHERE user_id = ? AND account_id = ?
		ORDER BY id`, userID, accountID)
	if err != nil {
		return nil, err
	}
	photos := []AccountPhoto{}
	for rows.Next() {
		var p AccountPhoto
		if err := rows.Scan(&p.ID, &p.UserID, &p.AccountID, &p.Path, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(photos) == 0 {
		return nil, nil
	}
	idx := cursor % len(photos)
	if idx < 0 {
		idx = 0
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE accounts SET photo_pool_cursor = ? WHERE id = ? AND user_id = ?`,
		(idx+1)%len(photos), accountID, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &photos[idx], nil
}
