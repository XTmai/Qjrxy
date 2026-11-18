// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package secure

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "scrypt$") {
		t.Errorf("哈希格式不符：%s", hash)
	}
	if strings.Contains(hash, "secret123") {
		t.Error("哈希不应包含明文")
	}
	if !VerifyPassword("secret123", hash) {
		t.Error("正确密码应校验通过")
	}
	if VerifyPassword("secret124", hash) {
		t.Error("错误密码不应通过")
	}
}

func TestHashPasswordUnique(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Error("相同密码应产生不同哈希（随机盐）")
	}
	if !VerifyPassword("same", a) || !VerifyPassword("same", b) {
		t.Error("两个哈希都应校验通过")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, bad := range []string{"", "plain", "scrypt$1$2$3", "bcrypt$x$y$z$a$b"} {
		if VerifyPassword("x", bad) {
			t.Errorf("格式错误的哈希不应通过：%q", bad)
		}
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plain := "校园网密码 password!@# 中文"
	enc, err := Encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain || strings.Contains(enc, "password") {
		t.Error("密文不应包含明文")
	}
	dec, err := Decrypt(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Errorf("往返不一致：%q != %q", dec, plain)
	}
}

func TestEncryptEmpty(t *testing.T) {
	key := make([]byte, 32)
	enc, err := Encrypt(key, "")
	if err != nil || enc != "" {
		t.Errorf("空字符串应返回空结果：%q %v", enc, err)
	}
	if dec, err := Decrypt(key, ""); err != nil || dec != "" {
		t.Errorf("空密文应返回空结果：%q %v", dec, err)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1
	enc, err := Encrypt(key1, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(key2, enc); err == nil {
		t.Error("错误密钥应解密失败")
	}
	if _, err := Decrypt(key1, "not-base64!!"); err == nil {
		t.Error("非法 base64 应报错")
	}
}

func TestNewToken(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok := NewToken()
		if tok == "" {
			t.Fatal("token 不应为空")
		}
		if seen[tok] {
			t.Fatal("token 重复")
		}
		seen[tok] = true
	}
}
