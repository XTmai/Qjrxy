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
