package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Account 是绑定的今日校园账号。
type Account struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId"`
	Alias       string `json:"alias"`
	SchoolName  string `json:"schoolName"`
	Username    string `json:"username"`
	PasswordEnc string `json:"-"`
	// TGCEnc 是登录时拿到的 CAS 票（tgc）的密文。票过期后用它重新换取
	// 业务会话，是短信登录（无密码）唯一的自动恢复手段。
	TGCEnc      string `json:"-"`
	TenantID    string `json:"tenantId"`
	Host        string `json:"host"`
	IDSURL      string `json:"idsUrl"`
	JoinType    string `json:"joinType"`
	Cookies     string `json:"-"`
	LoginStatus string `json:"loginStatus"`
	LastLoginAt string `json:"lastLoginAt"`
	// LastProbeAt / LastProbeMsg 记录最近一次登录态探测的结果，供前端展示。
	LastProbeAt   string `json:"lastProbeAt"`
	LastProbeMsg  string `json:"lastProbeMsg"`
	DeviceID      string `json:"deviceId"`
	DeviceModel   string `json:"deviceModel"`
	SystemVersion string `json:"systemVersion"`
	Platform      string `json:"platform"`    // android / ios
	WisDeviceID   string `json:"wisDeviceId"` // iOS 设备指纹头
	// UserAgent 是扫码时 App WebView 的原始 UA，提交时会原样使用，
	// 保证请求特征与真实 App 一致。
	UserAgent string `json:"userAgent"`
	// AppVersion 扫码时上报的今日校园客户端版本，用于 UA 与设备信息
	AppVersion string  `json:"appVersion"`
	BuildID    string  `json:"buildId"`
	Address    string  `json:"address"`
	Lon        float64 `json:"lon"`
	Lat        float64 `json:"lat"`
	Abnormal   string  `json:"abnormalReason"`
	PhotoPath  string  `json:"photoPath"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// ErrDuplicateAccount 表示同一学校下该学号已绑定。
// 调用方据此区分「重复添加」与真正的写入失败，避免把已成功的账号报成失败。
var ErrDuplicateAccount = errors.New("该学校下已绑定这个账号")

// HasCookie 表示账号是否保存了登录态。
func (a Account) HasCookie() bool {
	return strings.TrimSpace(a.Cookies) != ""
}

const accountColumns = `id, user_id, alias, school_name, username, password_enc, tgc_enc, tenant_id, host, ids_url,
	join_type, cookies, login_status, last_login_at, last_probe_at, last_probe_msg,
	device_id, device_model, system_version, platform,
	wis_device_id, user_agent, app_version, build_id, address, lon, lat, abnormal, photo_path,
	created_at, updated_at`

func scanAccount(scanner interface{ Scan(...any) error }) (*Account, error) {
	var a Account
	err := scanner.Scan(&a.ID, &a.UserID, &a.Alias, &a.SchoolName, &a.Username, &a.PasswordEnc, &a.TGCEnc,
		&a.TenantID, &a.Host, &a.IDSURL, &a.JoinType, &a.Cookies, &a.LoginStatus, &a.LastLoginAt,
		&a.LastProbeAt, &a.LastProbeMsg,
		&a.DeviceID, &a.DeviceModel, &a.SystemVersion, &a.Platform, &a.WisDeviceID, &a.UserAgent,
		&a.AppVersion, &a.BuildID, &a.Address, &a.Lon, &a.Lat, &a.Abnormal, &a.PhotoPath,
		&a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateAccount 新增账号。
func (db *DB) CreateAccount(ctx context.Context, a *Account) (*Account, error) {
	ts := now()
	a.CreatedAt, a.UpdatedAt = ts, ts
	if a.JoinType == "" {
		a.JoinType = "CAS"
	}
	if a.SystemVersion == "" {
		a.SystemVersion = "14"
	}
	if a.DeviceModel == "" {
		a.DeviceModel = "23127PN0CC"
	}
	if a.BuildID == "" {
		a.BuildID = "VLFCNXM"
	}
	if a.LoginStatus == "" {
		a.LoginStatus = "unknown"
	}
	if a.Platform == "" {
		a.Platform = "android"
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO accounts (user_id, alias, school_name, username, password_enc, tgc_enc, tenant_id, host, ids_url,
			join_type, cookies, login_status, last_login_at, device_id, device_model, system_version,
			platform, wis_device_id, user_agent, app_version, build_id, address, lon, lat, abnormal,
			photo_path, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.UserID, a.Alias, a.SchoolName, a.Username, a.PasswordEnc, a.TGCEnc, a.TenantID, a.Host, a.IDSURL,
		a.JoinType, a.Cookies, a.LoginStatus, a.LastLoginAt, a.DeviceID, a.DeviceModel, a.SystemVersion,
		a.Platform, a.WisDeviceID, a.UserAgent, a.AppVersion, a.BuildID, a.Address, a.Lon, a.Lat,
		a.Abnormal, a.PhotoPath, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicateAccount
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	a.ID = id
	return a, nil
}

// ListAccounts 返回用户全部账号。
func (db *DB) ListAccounts(ctx context.Context, userID int64) ([]Account, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// AccountByID 查询单个账号（带用户归属校验）。
func (db *DB) AccountByID(ctx context.Context, userID, id int64) (*Account, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id = ? AND user_id = ?`, id, userID)
	return scanAccount(row)
}

// AccountByIDInternal 不校验归属（调度器使用）。
func (db *DB) AccountByIDInternal(ctx context.Context, id int64) (*Account, error) {
	row := db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id)
	return scanAccount(row)
}

// AccountBySchoolUsername 按「用户 + 学校 + 学号」查询账号，用于重复添加时的复用。
func (db *DB) AccountBySchoolUsername(ctx context.Context, userID int64, schoolName, username string) (*Account, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = ? AND school_name = ? AND username = ?`,
		userID, schoolName, username)
	return scanAccount(row)
}

// UpdateAccount 更新账号可变字段。
func (db *DB) UpdateAccount(ctx context.Context, a *Account) error {
	a.UpdatedAt = now()
	_, err := db.ExecContext(ctx, `
		UPDATE accounts SET alias = ?, school_name = ?, username = ?, password_enc = ?, tgc_enc = ?, tenant_id = ?,
			host = ?, ids_url = ?, join_type = ?, cookies = ?, login_status = ?, last_login_at = ?,
			device_id = ?, device_model = ?, system_version = ?, platform = ?, wis_device_id = ?,
			user_agent = ?, app_version = ?, build_id = ?, address = ?, lon = ?, lat = ?,
			abnormal = ?, photo_path = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		a.Alias, a.SchoolName, a.Username, a.PasswordEnc, a.TGCEnc, a.TenantID, a.Host, a.IDSURL, a.JoinType,
		a.Cookies, a.LoginStatus, a.LastLoginAt, a.DeviceID, a.DeviceModel, a.SystemVersion, a.Platform,
		a.WisDeviceID, a.UserAgent, a.AppVersion, a.BuildID, a.Address, a.Lon, a.Lat, a.Abnormal,
		a.PhotoPath, a.UpdatedAt, a.ID, a.UserID)
	return err
}

// UpdateAccountSession 只更新登录态相关字段。
func (db *DB) UpdateAccountSession(ctx context.Context, id int64, cookies, status string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE accounts SET cookies = ?, login_status = ?, last_login_at = ?, updated_at = ? WHERE id = ?`,
		cookies, status, now(), now(), id)
	return err
}

// UpdateAccountTGC 只更新续期票据。
//
// 不能拿账号结构体整体 UpdateAccount：导入时先写 cookies、再写 tgc，
// 结构体里的 cookies 还是创建时的旧值（空），会把刚写入的登录态覆盖掉。
func (db *DB) UpdateAccountTGC(ctx context.Context, id int64, tgcEnc string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE accounts SET tgc_enc = ?, updated_at = ? WHERE id = ?`,
		tgcEnc, now(), id)
	return err
}

// RecordProbe 记录一次登录态探测的结果。
func (db *DB) RecordProbe(ctx context.Context, id int64, status, message string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE accounts SET login_status = ?, last_probe_at = ?, last_probe_msg = ?, updated_at = ?
		 WHERE id = ?`, status, now(), message, now(), id)
	return err
}

// ListAllAccounts 列出所有用户的账号（供后台批量探测使用）。
func (db *DB) ListAllAccounts(ctx context.Context) ([]Account, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// DeleteAccount 删除账号（级联删除任务/计划/日志）。
func (db *DB) DeleteAccount(ctx context.Context, userID, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
