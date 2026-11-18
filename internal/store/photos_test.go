// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package store

import (
	"context"
	"errors"
	"testing"
)

func TestAccountPhotoRotation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := mustUser(t, db)

	acc, err := db.CreateAccount(ctx, &Account{
		UserID: u.ID, SchoolName: "测试大学", Username: "2023001", Host: "t.campusphere.net",
	})
	if err != nil {
		t.Fatal(err)
	}

	var ids []int64
	for _, p := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		photo := &AccountPhoto{UserID: u.ID, AccountID: acc.ID, Path: "1_" + p}
		if err := db.CreateAccountPhoto(ctx, photo); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, photo.ID)
	}

	list, err := db.ListAccountPhotos(ctx, u.ID, acc.ID)
	if err != nil || len(list) != 3 {
		t.Fatalf("列表异常：%v %d", err, len(list))
	}
	for i, p := range list {
		if p.ID != ids[i] {
			t.Errorf("顺序应为上传顺序，第 %d 张得到 %d", i, p.ID)
		}
	}

	// 顺序轮换：A → B → C → A
	want := []int64{ids[0], ids[1], ids[2], ids[0]}
	for i, id := range want {
		got, err := db.NextAccountPhoto(ctx, u.ID, acc.ID)
		if err != nil {
			t.Fatalf("第 %d 次取图失败：%v", i, err)
		}
		if got == nil || got.ID != id {
			t.Fatalf("第 %d 次应取 %d，得到 %+v", i, id, got)
		}
	}

	stats, err := db.PhotoPoolStats(ctx, u.ID, acc.ID)
	if err != nil || stats.Count != 3 || stats.FirstID != ids[0] {
		t.Errorf("汇总异常：%+v err=%v", stats, err)
	}
	all, err := db.PhotoPoolStatsByUser(ctx, u.ID)
	if err != nil || all[acc.ID].Count != 3 {
		t.Errorf("批量汇总异常：%+v err=%v", all, err)
	}

	// 删除中间一张后数量与轮换都应以剩余照片为准
	path, err := db.DeleteAccountPhoto(ctx, u.ID, acc.ID, ids[1])
	if err != nil || path == "" {
		t.Fatalf("删除失败：%q %v", path, err)
	}
	if n, _ := db.CountAccountPhotos(ctx, u.ID, acc.ID); n != 2 {
		t.Errorf("删除后应为 2 张，得到 %d", n)
	}
	if _, err := db.AccountPhotoByID(ctx, u.ID, acc.ID, ids[1]); !errors.Is(err, ErrNotFound) {
		t.Errorf("已删除照片应查不到，得到 %v", err)
	}

	// 删除账号后池记录级联清空
	if err := db.DeleteAccount(ctx, u.ID, acc.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountAccountPhotos(ctx, u.ID, acc.ID); n != 0 {
		t.Errorf("账号删除后池应清空，剩余 %d", n)
	}
	if p, err := db.NextAccountPhoto(ctx, u.ID, acc.ID); !errors.Is(err, ErrNotFound) || p != nil {
		t.Errorf("账号不存在时取图应返回 ErrNotFound，得到 %+v %v", p, err)
	}
}
