package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"qjrxy/internal/store"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uploadPoolPhotos(t *testing.T, env *testEnv, accountID int64, names ...string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	data := testPNG(t)
	for _, name := range names {
		fw, err := w.CreateFormFile("photos", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/accounts/%d/photos", env.srv.URL, accountID), &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// TestPhotoPoolAPI 覆盖上传、列表、封面、缩略图/原图、删除与账号清理。
func TestPhotoPoolAPI(t *testing.T) {
	env := newSingleEnv(t)
	ctx := context.Background()
	env.do(t, http.MethodGet, "/api/auth/me", nil, "")
	user, err := env.db.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	acc, err := env.db.CreateAccount(ctx, &store.Account{
		UserID: user.ID, SchoolName: "测试大学", Username: "2023001", Host: "t.campusphere.net",
	})
	if err != nil {
		t.Fatal(err)
	}

	status, body := uploadPoolPhotos(t, env, acc.ID, "1.png", "2.png")
	if status != http.StatusOK {
		t.Fatalf("上传应 200，得到 %d %v", status, body)
	}
	data, _ := body["data"].(map[string]any)
	photos, _ := data["photos"].([]any)
	failed, _ := data["failed"].([]any)
	if len(photos) != 2 || len(failed) != 0 {
		t.Fatalf("应上传 2 张，得到 photos=%d failed=%v", len(photos), failed)
	}
	first, _ := photos[0].(map[string]any)
	photoURL, _ := first["url"].(string)
	if photoURL == "" {
		t.Fatalf("照片视图缺少 url：%v", first)
	}

	// 列表接口与账号列表的池数量/封面
	_, listBody := env.do(t, http.MethodGet, fmt.Sprintf("/api/accounts/%d/photos", acc.ID), nil, "")
	listData, _ := listBody["data"].(map[string]any)
	if got, _ := listData["photos"].([]any); len(got) != 2 {
		t.Fatalf("列表应有 2 张，得到 %v", listData)
	}
	_, accBody := env.do(t, http.MethodGet, "/api/accounts", nil, "")
	accData, _ := accBody["data"].(map[string]any)
	accList, _ := accData["accounts"].([]any)
	var view map[string]any
	for _, item := range accList {
		m, _ := item.(map[string]any)
		if int64(m["id"].(float64)) == acc.ID {
			view = m
		}
	}
	if view == nil {
		t.Fatal("账号列表未找到测试账号")
	}
	if view["photoPoolCount"].(float64) != 2 || view["photoUrl"] != photoURL {
		t.Errorf("列表应带池数量与封面：%v", view)
	}

	// 原图可访问
	resp, err := http.Get(env.srv.URL + photoURL)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(raw) == 0 {
		t.Errorf("照片读取失败：%d bytes=%d", resp.StatusCode, len(raw))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("应为原图 image/png，得到 %q", ct)
	}

	// 非法格式逐文件失败，不影响已成功的
	status, body = uploadPoolPhotos(t, env, acc.ID, "bad.gif")
	if status != http.StatusOK {
		t.Fatalf("含非法文件的上传仍应 200，得到 %d", status)
	}
	data, _ = body["data"].(map[string]any)
	failed, _ = data["failed"].([]any)
	if len(failed) != 1 {
		t.Errorf("非法扩展名应返回 1 条失败，得到 %v", data)
	}

	// 跨账号访问同一 photoId 应 404
	other, err := env.db.CreateAccount(ctx, &store.Account{
		UserID: user.ID, SchoolName: "测试大学", Username: "2023002", Host: "t.campusphere.net",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/accounts/%d/photos/%d", env.srv.URL, other.ID, int64(first["id"].(float64))), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("跨账号访问应 404，得到 %d", resp.StatusCode)
	}

	// 删除一张：列表剩 1，文件被清理
	status, _ = env.do(t, http.MethodDelete,
		fmt.Sprintf("/api/accounts/%d/photos/%d", acc.ID, int64(first["id"].(float64))), nil, "")
	if status != http.StatusOK {
		t.Errorf("删除照片应 200，得到 %d", status)
	}
	_, listBody = env.do(t, http.MethodGet, fmt.Sprintf("/api/accounts/%d/photos", acc.ID), nil, "")
	listData, _ = listBody["data"].(map[string]any)
	if got, _ := listData["photos"].([]any); len(got) != 1 {
		t.Errorf("删除后应剩 1 张，得到 %v", listData)
	}

	// 删除账号：池记录级联删除，照片目录整体清理
	photoDir := filepath.Join(env.svc.Config().PhotosDir(), fmt.Sprintf("%d_%d", user.ID, acc.ID))
	if _, err := os.Stat(photoDir); err != nil {
		t.Fatalf("上传后照片目录应存在：%v", err)
	}
	status, _ = env.do(t, http.MethodDelete, fmt.Sprintf("/api/accounts/%d", acc.ID), nil, "")
	if status != http.StatusOK {
		t.Fatalf("删除账号应 200，得到 %d", status)
	}
	if _, err := os.Stat(photoDir); !os.IsNotExist(err) {
		t.Errorf("删除账号后照片目录应消失，stat err=%v", err)
	}
}
