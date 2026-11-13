package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// RedeemCode 是一张充值卡密。
type RedeemCode struct {
	Code      string `json:"code"`
	Amount    int    `json:"amount"`
	UsedBy    int64  `json:"usedBy"`
	UsedAt    string `json:"usedAt"`
	CreatedAt string `json:"createdAt"`
}

// RedeemCode 消费一张卡密并返回面额；卡密不存在或已被使用返回 ErrNotFound。
//
// 用「条件 UPDATE + 影响行数」做原子占用：并发兑换同一个码时，
// 只有第一个请求能把 used_by 从 NULL 改掉，其余拿到 0 行而失败。
// 早期实现只校验格式就固定返回 30，任意 QJ-XXXX-XXXX 都能无限次兑换。
//
// 注意：这个方法只烧码不加额度。加额度请用 RedeemAndGrant——
// 两者分开的话，烧码提交后加额度失败，卡密就白白作废了。
func (db *DB) RedeemCode(ctx context.Context, code string, userID int64) (int, error) {
	code = normalizeCode(code)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE redeem_codes SET used_by = ?, used_at = ? WHERE code = ? AND used_by IS NULL`,
		userID, now(), code)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}

	var amount int
	err = tx.QueryRowContext(ctx, `SELECT amount FROM redeem_codes WHERE code = ?`, code).Scan(&amount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return amount, nil
}

// RedeemAndGrant 在同一个事务里消费卡密并给用户加额度。
//
// 之前的实现分两步：RedeemCode 的事务先提交（卡密标记已用），
// GrantQuota 是另一条独立 UPDATE。两步之间一旦失败（进程退出、DB 抖动），
// 卡密已被烧掉、额度没加上，且重试永远返回「卡密无效或已被使用」。
// 现在加额度和烧码同生共死：任一步失败整体回滚，卡密回到未使用状态。
func (db *DB) RedeemAndGrant(ctx context.Context, code string, userID int64) (int, error) {
	code = normalizeCode(code)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE redeem_codes SET used_by = ?, used_at = ? WHERE code = ? AND used_by IS NULL`,
		userID, now(), code)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}

	var amount int
	err = tx.QueryRowContext(ctx, `SELECT amount FROM redeem_codes WHERE code = ?`, code).Scan(&amount)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if amount <= 0 {
		return 0, fmt.Errorf("卡密面额非法（%d）", amount)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET quota = quota + ? WHERE id = ?`, amount, userID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return amount, nil
}

// CreateRedeemCode 预置一张卡密（发卡前先入库）。
func (db *DB) CreateRedeemCode(ctx context.Context, code string, amount int) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO redeem_codes (code, amount, used_by, used_at, created_at)
		 VALUES (?, ?, NULL, '', ?)`,
		normalizeCode(code), amount, now())
	return err
}

// RedeemCodeByCode 查询单张卡密（诊断用）。
func (db *DB) RedeemCodeByCode(ctx context.Context, code string) (*RedeemCode, error) {
	row := db.QueryRowContext(ctx,
		`SELECT code, amount, COALESCE(used_by, 0), used_at, created_at FROM redeem_codes WHERE code = ?`,
		normalizeCode(code))
	var c RedeemCode
	err := row.Scan(&c.Code, &c.Amount, &c.UsedBy, &c.UsedAt, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
