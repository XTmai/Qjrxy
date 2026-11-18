// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import "testing"

// 计划指定日期：格式必须合法、去重排序、上限保护。
func TestNormalizeDates(t *testing.T) {
	got, err := normalizeDates([]string{"2026-11-20", " 2026-11-19 ", "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "2026-11-19" || got[1] != "2026-11-20" {
		t.Fatalf("去重排序不符：%v", got)
	}

	if _, err := normalizeDates([]string{"2026/11/20"}); err == nil {
		t.Fatal("非法格式应报错")
	}

	empty, err := normalizeDates([]string{"", "  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("空输入应返回空数组（按星期执行），得到 %v", empty)
	}
}
