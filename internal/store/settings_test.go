// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package store_test

import (
	"testing"

	"qjrxy/internal/store"
)

func TestSettingsRoundTrip(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()

	// 未配置时读取不报错且为空
	if v, err := db.GetSetting(ctx, "none"); err != nil || v != "" {
		t.Fatalf("未配置键应返回空串，得到 %q, err=%v", v, err)
	}
	if err := db.SetSetting(ctx, "diag.enabled", "1"); err != nil {
		t.Fatal(err)
	}
	// upsert 覆盖
	if err := db.SetSetting(ctx, "diag.enabled", "0"); err != nil {
		t.Fatal(err)
	}
	v, err := db.GetSetting(ctx, "diag.enabled")
	if err != nil || v != "0" {
		t.Fatalf("diag.enabled = %q, err=%v，期望 \"0\"", v, err)
	}
	all, err := db.AllSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all["diag.enabled"] != "0" {
		t.Fatalf("AllSettings 未包含覆盖后的值：%v", all)
	}
}
