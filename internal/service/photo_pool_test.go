// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"qjrxy/internal/store"
)

func TestSavePoolPhotoValidation(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	rel, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, "dorm.JPG", []byte("fake-jpeg"))
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := fmt.Sprintf("%d_%d/pool/", user.ID, acc.ID)
	if !strings.HasPrefix(rel, wantPrefix) {
		t.Errorf("池照片应放在 pool/ 子目录，得到 %q", rel)
	}
	if _, err := os.Stat(svc.PhotoAbsPath(rel)); err != nil {
		t.Errorf("文件未落盘：%v", err)
	}

	if _, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, "x.gif", []byte("x")); err == nil {
		t.Error("不支持的扩展名应被拒绝")
	}
	if _, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, "x.jpg", nil); err == nil {
		t.Error("空文件应被拒绝")
	}
	if _, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, "x.jpg", make([]byte, 5<<20+1)); err == nil {
		t.Error("超过 5MB 应被拒绝")
	}
}

func TestSavePoolPhotoGlobalQuota(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	if err := svc.DB().SetSetting(ctx, PhotoPoolLimitKey, strconv.FormatInt(1024, 10)); err != nil {
		t.Fatal(err)
	}
	if got := svc.PhotoPoolLimit(ctx); got != 1024 {
		t.Fatalf("上限读取异常：%d", got)
	}
	if _, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, "a.jpg", make([]byte, 600)); err != nil {
		t.Fatalf("首张应能上传：%v", err)
	}
	if _, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, "b.jpg", make([]byte, 600)); err == nil {
		t.Fatal("超过全局容量应被拒绝")
	} else if !strings.Contains(err.Error(), "容量已满") {
		t.Errorf("错误信息应说明容量已满：%v", err)
	}
	used, count := svc.PhotoPoolUsage()
	if used != 600 || count != 1 {
		t.Errorf("用量统计异常：used=%d count=%d", used, count)
	}
}

func TestRunPhotoPoolRotationAndFallback(t *testing.T) {
	svc, user := newTestService(t)
	acc := newTestAccount(t, svc, user.ID)
	ctx := context.Background()

	addPool := func(name string) string {
		t.Helper()
		rel, err := svc.SavePoolPhoto(ctx, user.ID, acc.ID, name, []byte("fake"))
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.DB().CreateAccountPhoto(ctx, &store.AccountPhoto{
			UserID: user.ID, AccountID: acc.ID, Path: rel,
		}); err != nil {
			t.Fatal(err)
		}
		return rel
	}
	first := addPool("1.jpg")
	second := addPool("2.jpg")

	// 池优先，按上传顺序轮换：1 → 2 → 1
	for i, want := range []string{first, second, first} {
		got, fromPool := svc.runPhoto(ctx, acc)
		if !fromPool || got != svc.PhotoAbsPath(want) {
			t.Fatalf("第 %d 次应取池内 %q，得到 %q (pool=%v)", i, want, got, fromPool)
		}
	}

	// 文件丢失的池记录应被自愈删除，然后继续取剩余照片
	if err := os.Remove(svc.PhotoAbsPath(second)); err != nil {
		t.Fatal(err)
	}
	got, fromPool := svc.runPhoto(ctx, acc)
	if !fromPool || got != svc.PhotoAbsPath(first) {
		t.Fatalf("坏图应跳过，得到 %q (pool=%v)", got, fromPool)
	}
	if n, _ := svc.DB().CountAccountPhotos(ctx, user.ID, acc.ID); n != 1 {
		t.Errorf("坏图记录应被删除，剩余 %d 条", n)
	}

	// 池空后回退单张
	single, err := svc.SavePhoto(user.ID, acc.ID, "single.jpg", []byte("fake"))
	if err != nil {
		t.Fatal(err)
	}
	acc.PhotoPath = single
	if _, err := svc.DB().DeleteAccountPhoto(ctx, user.ID, acc.ID, 1); err != nil {
		t.Fatal(err)
	}
	got, fromPool = svc.runPhoto(ctx, acc)
	if fromPool || got != svc.PhotoAbsPath(single) {
		t.Errorf("池空应回退单张，得到 %q (pool=%v)", got, fromPool)
	}

	// 都没有 → 空
	acc.PhotoPath = ""
	if got, fromPool := svc.runPhoto(ctx, acc); got != "" || fromPool {
		t.Errorf("无照片时应返回空，得到 %q (pool=%v)", got, fromPool)
	}

	// 账号目录清理
	svc.RemoveAccountPhotoDir(user.ID, acc.ID)
	dir := filepath.Join(svc.Config().PhotosDir(), fmt.Sprintf("%d_%d", user.ID, acc.ID))
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("账号照片目录应被删除，stat err=%v", err)
	}
}
