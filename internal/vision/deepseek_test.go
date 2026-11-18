// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package vision

import (
	"reflect"
	"testing"
)

// 模型的回答并不总是干净的 JSON：可能带围栏、带前后说明、混入越界值。
func TestParseCells(t *testing.T) {
	cases := []struct {
		name  string
		input string
		total int
		want  []int
	}{
		{"纯数组", "[2,5]", 9, []int{2, 5}},
		{"带围栏", "```json\n[2,5]\n```", 9, []int{2, 5}},
		{"带说明", "包含目标物的格子是：[1, 4, 7]", 9, []int{1, 4, 7}},
		{"字符串数字", `["3","6"]`, 9, []int{3, 6}},
		{"去重排序", "[5,2,5,1]", 9, []int{1, 2, 5}},
		{"越界丢弃", "[0,3,10]", 9, []int{3}},
		{"空数组", "[]", 9, []int{}},
		{"纯文本兜底", "第2格和第8格有", 9, []int{2, 8}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseCells(c.input, c.total)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parseCells(%q) = %v，期望 %v", c.input, got, c.want)
			}
		})
	}
}

// total 小于 9 时，编号必须按实际图数收敛，不能把不存在的格子交回去。
func TestParseCellsRespectsTotal(t *testing.T) {
	if got := parseCells("[1,5,9]", 5); !reflect.DeepEqual(got, []int{1, 5}) {
		t.Fatalf("total=5 时应丢弃 9，实际 %v", got)
	}
}
