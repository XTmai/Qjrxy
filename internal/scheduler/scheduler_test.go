package scheduler

import (
	"testing"
	"time"

	"qingjin/internal/store"
)

func TestParseClock(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"00:00", 0, true},
		{"23:59", 23*60 + 59, true},
		{"20:05", 20*60 + 5, true},
		{"24:00", 0, false},
		{"12:60", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseClock(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseClock(%q) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestInWindow(t *testing.T) {
	at := func(h, m int) time.Time {
		return time.Date(2026, 11, 1, h, m, 0, 0, time.Local)
	}
	cases := []struct {
		now        time.Time
		start, end string
		want       bool
	}{
		{at(20, 30), "20:00", "23:30", true},
		{at(19, 59), "20:00", "23:30", false},
		{at(23, 31), "20:00", "23:30", false},
		// 跨天区间
		{at(23, 30), "22:00", "02:00", true},
		{at(1, 0), "22:00", "02:00", true},
		{at(12, 0), "22:00", "02:00", false},
		// 非法配置时不限制
		{at(12, 0), "bad", "worse", true},
	}
	for _, c := range cases {
		if got := inWindow(c.now, c.start, c.end); got != c.want {
			t.Errorf("inWindow(%s, %s-%s) = %v want %v",
				c.now.Format("15:04"), c.start, c.end, got, c.want)
		}
	}
}

func TestShouldRun(t *testing.T) {
	// 2026-11-01 是周日 -> 星期索引 6
	sunday := time.Date(2026, 11, 1, 20, 0, 0, 0, time.Local)
	monday := time.Date(2026, 11, 2, 20, 0, 0, 0, time.Local)

	base := store.Plan{Enabled: true, Mode: "school", Days: []int{6}}

	if !shouldRun(base, sunday) {
		t.Error("周日 + 选中周日 应执行")
	}
	if shouldRun(base, monday) {
		t.Error("周一不在执行日期内，不应执行")
	}

	// 已停用
	disabled := base
	disabled.Enabled = false
	if shouldRun(disabled, sunday) {
		t.Error("停用计划不应执行")
	}

	// 当天已执行
	ran := base
	ran.LastRunAt = "2026-11-01T20:00:00+08:00"
	if shouldRun(ran, sunday) {
		t.Error("当天已执行过不应重复")
	}

	// 昨天执行过可以再跑
	ranYesterday := base
	ranYesterday.LastRunAt = "2026-10-31T20:00:00+08:00"
	if !shouldRun(ranYesterday, sunday) {
		t.Error("昨天执行过不应阻止今天执行")
	}

	// 自定义时段外不执行
	custom := store.Plan{Enabled: true, Mode: "custom", Days: []int{6},
		WindowStart: "21:00", WindowEnd: "23:00"}
	if shouldRun(custom, sunday) {
		t.Error("20:00 不在 21:00-23:00 内，不应执行")
	}
	custom.WindowStart, custom.WindowEnd = "19:00", "21:00"
	if !shouldRun(custom, sunday) {
		t.Error("20:00 应在 19:00-21:00 内")
	}
}
