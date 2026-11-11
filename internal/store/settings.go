// settings 表的读写：admin 页可改的运行时配置都落在这里
// （诊断开关、识别服务参数等）。表结构极简（key/value），
// 值由调用方负责序列化与加密（如 API key 先过 secure.Encrypt）。
package store

import "context"

// GetSetting 读取单个配置项；不存在时返回 ("", nil)。
func (db *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		// 没配过不算错误，调用方按默认值处理
		return "", nil
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
