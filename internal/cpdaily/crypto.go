// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package cpdaily

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// aesIV 是签到提交体固定的初始向量（与 App 原生实现一致）。
var aesIV = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}

// desIV 是设备扩展头固定的初始向量。
var desIV = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

// AESEncryptCBC 使用 AES-128-CBC + PKCS5 填充加密，返回 base64。
// 用于签到提交体 bodyString（key = FKeyCat）。
func AESEncryptCBC(plaintext, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	padded := pkcs5Pad([]byte(plaintext), block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, aesIV).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

// AESDecryptCBC 解密 AES-128-CBC + PKCS5，用于自检与调试。
func AESDecryptCBC(ciphertext, key string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || len(raw)%block.BlockSize() != 0 {
		return "", errors.New("密文长度不是块大小的整数倍")
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, aesIV).CryptBlocks(out, raw)
	out, err = pkcs5Unpad(out)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// AESEncryptECB 是旧协议（app 9.9.11 之前）的提交加密方式，保留用于兼容老服务端。
func AESEncryptECB(plaintext, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	padded := pkcs5Pad([]byte(plaintext), block.BlockSize())
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(out[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// DESEncrypt 使用 DES-CBC + PKCS5 加密，返回 base64，对齐 App 的 pyDes 实现。
func DESEncrypt(plaintext, key string) (string, error) {
	block, err := des.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	padded := pkcs5Pad([]byte(plaintext), block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, desIV).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

func pkcs5Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func pkcs5Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("空数据")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > len(data) {
		return nil, errors.New("填充无效")
	}
	return data[:len(data)-pad], nil
}

// MD5Hex 返回小写十六进制摘要。
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Interleave 密钥派生：偶数位在前、奇数位在后（对应 App 的 getak）。
func Interleave(s string) string {
	var even, odd strings.Builder
	for i, r := range s {
		if i%2 == 0 {
			even.WriteRune(r)
		} else {
			odd.WriteRune(r)
		}
	}
	return even.String() + odd.String()
}

// SecretKeySalt 是 getSecretKey 的签名盐（sha256("hello")）。
const SecretKeySalt = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

const secretPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCpwqE77zx+WexQWf1qDzb8EGey
et7VO78JiCDrZLcesaGjN5B8s66bMCfcn9R3GN25/EA3lFPI86hgluMPT6X1rneW
QRdKMaFWtxZLc+0Gdkf8zvdPf3rcYt6VjPnq3Hu1iSdytD8SIuTcSGFoPctId7jZ
zEjMjj74mnu3WtihZQIDAQAB
-----END PUBLIC KEY-----`

func parseRSAPublicKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(secretPublicKeyPEM))
	if block == nil {
		return nil, errors.New("公钥解析失败")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("不是 RSA 公钥")
	}
	return pub, nil
}

// RSAEncryptSecret 加密 getSecretKey 的 p 参数。
func RSAEncryptSecret(seed string) (string, error) {
	pub, err := parseRSAPublicKey()
	if err != nil {
		return "", err
	}
	raw, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(seed))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// CryptoSelfTest 校验加解密链路，供 /api/system/selftest 使用。
func CryptoSelfTest() error {
	const sample = `{"test":"今日校园"}`
	enc, err := AESEncryptCBC(sample, FKeyCat)
	if err != nil {
		return err
	}
	dec, err := AESDecryptCBC(enc, FKeyCat)
	if err != nil {
		return err
	}
	if dec != sample {
		return fmt.Errorf("AES 自检失败：得到 %q", dec)
	}
	if _, err := DESEncrypt(`{"deviceId":"selftest"}`, desKeyInfo); err != nil {
		return err
	}
	// 已知向量：interleave("h8r7uN8t"+"9hriwfra") 必须等于 FKeyCat。
	if got := Interleave("h8r7uN8t" + "9hriwfra"); got != FKeyCat {
		return fmt.Errorf("密钥派生自检失败：得到 %q", got)
	}
	return nil
}

// sha256Hex 便于调试输出。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------- IAP 网页版登录的 RSA 密码加密 ----------

// 网页版登录页把 RSA-1024 公钥写在 chunk-common.js 里，
// 密码格式为 "{rsa}" + base64(RSA_PKCS1v15(password))。
// 前缀是服务端识别加密方式的标记，缺了它服务端会按明文处理并返回 FAIL_UPNOTMATCH。
const rsaPasswordPrefix = "{rsa}"

// 常见学校的兜底公钥（多所学校共用同一把，但各校仍应优先用页面动态抓取）。
const fallbackIAPPublicKey = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDCpbRy8ZoyQvRPpUDIXycglTwVDcYr" +
	"Lcv7P9HI4E7/TAPZiI3GN0ckTUVVWRvoo/re3uCOOJK+F+Ufh19XiRIdoPYCULsSyEjcL" +
	"LiyAS09mVOzhAQco84E/lM24T7rRTVID7LIgWPewyN8Nd5vzHET4Q3qTthoL8n82BwC1i" +
	"XuVQIDAQAB"

// RSAEncryptIAPPassword 按网页版格式加密 IAP 登录密码。
// 返回形如 {rsa}K16e2jYBS... 的字符串。
func RSAEncryptIAPPassword(password, publicKeyB64 string) (string, error) {
	if publicKeyB64 == "" {
		publicKeyB64 = fallbackIAPPublicKey
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKeyB64))
	if err != nil {
		return "", fmt.Errorf("RSA 公钥 base64 解码失败：%w", err)
	}
	pubAny, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", fmt.Errorf("RSA 公钥解析失败：%w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("不是 RSA 公钥")
	}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(password))
	if err != nil {
		return "", err
	}
	return rsaPasswordPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// ExtractRSAPublicKey 从登录页 JS 里提取 SPKI/base64 形式的 RSA 公钥。
// 找不到时返回空串，由调用方回退到内置公钥。
func ExtractRSAPublicKey(js string) string {
	// 今日校园统一认证用的是 1024 位公钥，base64 前缀稳定为 MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ
	idx := strings.Index(js, "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ")
	if idx < 0 {
		idx = strings.Index(js, "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBg")
	}
	if idx < 0 {
		return ""
	}
	rest := js[idx:]
	end := 0
	for end < len(rest) {
		c := rest[end]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '+' || c == '/' || c == '=' {
			end++
			continue
		}
		break
	}
	key := rest[:end]
	if len(key) < 200 {
		return ""
	}
	return key
}
