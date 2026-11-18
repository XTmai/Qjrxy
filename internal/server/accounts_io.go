// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"qjrxy/internal/service"
)

// 账号导入导出。
//
// 导出默认带走登录态（cookies + tgc），导入到新机器后即可继续使用；
// 密码默认不含，显式 withPassword=1 才导出明文，前端会做二次确认。

// csvHeader 返回 CSV 表头（字段名与 JSON tag 一致，导入时可直接按列名解析）。
func csvHeader(withSession, withPassword bool) []string {
	cols := []string{
		"alias", "schoolName", "username",
		"deviceId", "deviceModel", "systemVersion", "platform",
		"wisDeviceId", "userAgent", "appVersion", "buildId",
		"address", "lon", "lat", "abnormalReason",
		"loginStatus", "lastLoginAt",
	}
	if withSession {
		cols = append(cols, "cookies", "tgc")
	}
	if withPassword {
		cols = append(cols, "password")
	}
	return cols
}

// handleExportAccounts 导出账号（json / csv）。
func (s *Server) handleExportAccounts(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		writeError(w, badRequest("format 仅支持 json 或 csv"))
		return
	}
	// 登录态默认导出（迁移后免登录），密码默认不导出。
	withSession := r.URL.Query().Get("withSession") != "0"
	withPassword := r.URL.Query().Get("withPassword") == "1"

	file, err := s.svc.ExportAccounts(r.Context(), user.ID, withSession, withPassword)
	if err != nil {
		writeError(w, err)
		return
	}

	suffix := ""
	if withSession {
		suffix += "-with-session"
	}
	if withPassword {
		suffix += "-with-password"
	}
	name := fmt.Sprintf("qjrxy-accounts-%s%s.%s", time.Now().Format("20060102-1504"), suffix, format)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		// Excel 需要 BOM 才按 UTF-8 解析，否则中文全是乱码。
		_, _ = w.Write([]byte("\uFEFF"))
		cw := csv.NewWriter(w)
		_ = cw.Write(csvHeader(withSession, withPassword))
		for _, a := range file.Accounts {
			row := []string{
				a.Alias, a.SchoolName, a.Username,
				a.DeviceID, a.DeviceModel, a.SystemVersion, a.Platform,
				a.WisDeviceID, a.UserAgent, a.AppVersion, a.BuildID,
				a.Address,
				strconv.FormatFloat(a.Lon, 'f', -1, 64),
				strconv.FormatFloat(a.Lat, 'f', -1, 64),
				a.Abnormal, a.LoginStatus, a.LastLoginAt,
			}
			if withSession {
				cookies := ""
				if len(a.Cookies) > 0 {
					if buf, err := json.Marshal(a.Cookies); err == nil {
						cookies = string(buf)
					}
				}
				row = append(row, cookies, a.TGC)
			}
			if withPassword {
				row = append(row, a.Password)
			}
			_ = cw.Write(row)
		}
		cw.Flush()
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(file)
}

// handleImportAccounts 导入账号（前端解析 CSV 后按 JSON 提交）。
func (s *Server) handleImportAccounts(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Accounts []service.ExportAccount `json:"accounts"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if len(req.Accounts) == 0 {
		writeError(w, badRequest("导入内容为空"))
		return
	}
	if len(req.Accounts) > 500 {
		writeError(w, badRequest("单次最多导入 500 个账号"))
		return
	}
	res := s.svc.ImportAccounts(r.Context(), user.ID, req.Accounts)
	writeData(w, http.StatusOK, res)
}
