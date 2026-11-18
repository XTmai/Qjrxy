// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// buildTestJPEG 生成指定尺寸的纯色 JPEG。
func buildTestJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeDims(t *testing.T, data []byte) (int, int) {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码缩略图失败: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

func TestPhotoThumbnailScalesDown(t *testing.T) {
	// 模拟手机横拍原图 1200×800
	thumb, err := photoThumbnailFromBytes(buildTestJPEG(t, 1200, 800, color.RGBA{R: 200, G: 30, B: 40, A: 255}), 96)
	if err != nil {
		t.Fatalf("生成缩略图失败: %v", err)
	}
	w, h := decodeDims(t, thumb)
	if w != 96 || h != 64 {
		t.Fatalf("期望 96×64，得到 %d×%d", w, h)
	}
	// 大幅缩小后应仍为主体红色（面积平均不会串色）
	img, _ := jpeg.Decode(bytes.NewReader(thumb))
	r, g, b, _ := img.At(48, 32).RGBA()
	if r < 150<<8 || g > 100<<8 || b > 100<<8 {
		t.Fatalf("主体颜色不应被改变: r=%d g=%d b=%d", r>>8, g>>8, b>>8)
	}
}

func TestPhotoThumbnailNoUpscale(t *testing.T) {
	_, err := photoThumbnailFromBytes(buildTestJPEG(t, 64, 48, color.RGBA{G: 255, A: 255}), 96)
	if err != errThumbnailUnsupported {
		t.Fatalf("小图不应生成缩略图，得到 err=%v", err)
	}
}

func TestJPEGOrientationParse(t *testing.T) {
	// 无 EXIF 的普通 JPEG
	if got := jpegOrientation(buildTestJPEG(t, 32, 32, color.RGBA{A: 255})); got != 1 {
		t.Fatalf("无 EXIF 应返回 1，得到 %d", got)
	}
}

func TestApplyOrientationRotates(t *testing.T) {
	// 2×1 横条（左红右绿），orientation=6（旋转 90° CW）→ 应变为 1×2 竖条，左端（红）去顶部
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})

	got := applyOrientation(img, 6)
	if got.Bounds().Dx() != 1 || got.Bounds().Dy() != 2 {
		t.Fatalf("orientation=6 应宽高互换，得到 %d×%d", got.Bounds().Dx(), got.Bounds().Dy())
	}
	// 直接对比像素数组（不经过 color.RGBA.RGBA() 转换）
	ra, ok := got.(*image.RGBA)
	if !ok {
		t.Fatalf("got 应为 *image.RGBA，得到 %T", got)
	}
	if got.At(0, 0) != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("顶部应为红色: %v", got.At(0, 0))
	}
	if got.At(0, 1) != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("底部应为绿色: %v", got.At(0, 1))
	}
	_ = ra
}
