// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

import "testing"

func TestIsDeviceAuthRequired(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		message string
		want    bool
	}{
		{"code=3 且更换手机文案", "3", "您疑似更换手机频繁，如需继续使用当前手机，请联系辅导员在辅导猫的手机授权管理功能中为您授权使用该手机", true},
		{"code 为空仅凭文案", "", "请联系辅导员在辅导猫的手机授权管理功能中为您授权使用该手机", true},
		{"code=3 但文案无关（不同学校 code 语义可能不同，不误判）", "3", "其他错误", false},
		{"普通失败", "-1", "提交失败", false},
		{"未到时间", "0", "查寝未开始", false},
		{"空输入", "", "", false},
	}
	for _, c := range cases {
		if got := IsDeviceAuthRequired(c.code, c.message); got != c.want {
			t.Errorf("%s: IsDeviceAuthRequired(%q, %q) = %v, 期望 %v", c.name, c.code, c.message, got, c.want)
		}
	}
}

func TestSanitizeSchoolMessage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"纯文本", "纯文本"},
		{"联系辅导员在辅导猫的<span style=\"color:red\">手机授权管理</span>功能中为您授权使用该手机",
			"联系辅导员在辅导猫的手机授权管理功能中为您授权使用该手机"},
		{"  提交失败  ", "提交失败"},
		{"a<br>b", "ab"},
		{"多  <b>段</b>  空格", "多 段 空格"},
	}
	for _, c := range cases {
		if got := sanitizeSchoolMessage(c.in); got != c.want {
			t.Errorf("sanitizeSchoolMessage(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}
