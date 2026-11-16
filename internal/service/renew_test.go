package service

import (
	"bytes"
	"context"
	"testing"

	"qjrxy/internal/config"
	"qjrxy/internal/store"
)

// 没有保存 CAS 票的账号（账密登录）不能走 tgc 续期，应快速失败而不是发请求。
func TestRenewSessionWithoutTGC(t *testing.T) {
	svc := &Service{cfg: &config.Config{SecretKey: bytes.Repeat([]byte{1}, 32)}}
	err := svc.renewSessionWithTGC(context.Background(), &store.Account{ID: 1})
	if err == nil {
		t.Fatal("无 TGC 时续期应报错")
	}
}

// 票据密文损坏时不能把垃圾送进换票流程。
func TestRenewSessionWithCorruptTGC(t *testing.T) {
	svc := &Service{cfg: &config.Config{SecretKey: bytes.Repeat([]byte{1}, 32)}}
	err := svc.renewSessionWithTGC(context.Background(), &store.Account{ID: 1, TGCEnc: "not-a-valid-ciphertext"})
	if err == nil {
		t.Fatal("损坏的 TGC 应报错")
	}
}

// TestKeepSchoolHostForSMS 校验 NOTCLOUD 学校不会被响应租户覆盖门户。
func TestKeepSchoolHostForSMS(t *testing.T) {
	cases := []struct {
		join, host string
		want       bool
	}{
		{"NOTCLOUD", "ahnu.campusphere.net", true},
		{"NOTCLOUD", "fyvtc.campusphere.net", true},
		{"NOTCLOUD", "", false},
		{"CLOUD", "kjxyctgu.campusphere.net", false},
		{"", "ahnu.campusphere.net", false},
	}
	for _, c := range cases {
		a := &store.Account{JoinType: c.join, Host: c.host}
		if got := keepSchoolHostForSMS(a); got != c.want {
			t.Errorf("keepSchoolHostForSMS(%q,%q)=%v, want %v", c.join, c.host, got, c.want)
		}
	}
}
