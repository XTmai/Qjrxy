package cpdaily

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

// 这份 JSON 是参考实现 documents/开发者文档.md 里记录的真实结构：
// result 下是会话码、目标词和 9 张候选图，图片每张带自己的 code。
const capRealCreateJSON = `{
  "code": "0",
  "message": "SUCCESS",
  "result": {
    "code": "0123456789abcdef",
    "name": "自行车",
    "imageInfos": [
      {"code":"a1","path":"http://cdn/1.jpg"},
      {"code":"a2","path":"http://cdn/2.jpg"},
      {"code":"a3","path":"http://cdn/3.jpg"},
      {"code":"a4","path":"http://cdn/4.jpg"},
      {"code":"a5","path":"http://cdn/5.jpg"},
      {"code":"a6","path":"http://cdn/6.jpg"},
      {"code":"a7","path":"http://cdn/7.jpg"},
      {"code":"a8","path":"http://cdn/8.jpg"},
      {"code":"a9","path":"http://cdn/9.jpg"}
    ]
  }
}`

func TestParseCaptchaCreateRealShape(t *testing.T) {
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(capRealCreateJSON), &out); err != nil {
		t.Fatal(err)
	}
	cr, err := parseCaptchaCreate(out)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if cr.Code != "0123456789abcdef" {
		t.Fatalf("会话码错误：%q", cr.Code)
	}
	if cr.Name != "自行车" {
		t.Fatalf("目标词错误：%q", cr.Name)
	}
	if len(cr.ImageInfos) != 9 {
		t.Fatalf("应解析出 9 张候选图，实际 %d", len(cr.ImageInfos))
	}
	if cr.ImageInfos[1].Code != "a2" || cr.ImageInfos[8].Code != "a9" {
		t.Fatalf("图片 code 顺序错乱：%+v", cr.ImageInfos)
	}
}

func TestComposeGrid(t *testing.T) {
	imgs := make([]image.Image, 9)
	for i := range imgs {
		// 每格一种颜色，便于定位
		imgs[i] = capSolidImage(80, 60, color.RGBA{R: uint8(20 * i), G: 140, B: 90, A: 255})
	}
	grid := composeGrid(imgs, 100)
	if grid.Bounds().Dx() != 300 || grid.Bounds().Dy() != 300 {
		t.Fatalf("九宫格尺寸应为 300×300，实际 %v", grid.Bounds())
	}
	// 第 1 格中心、第 9 格中心应各自有颜色，且不相同
	c1 := grid.RGBAAt(50, 50)
	c9 := grid.RGBAAt(250, 250)
	if c1 == c9 {
		t.Fatalf("第 1 格与第 9 格颜色相同，拼图位置可能重复：%v %v", c1, c9)
	}
	// 分割线应为浅灰
	if line := grid.RGBAAt(100, 10); line.R < 200 || line.R > 230 {
		t.Fatalf("分割线颜色异常：%v", line)
	}
}

func TestComposeGridLeavesMissingCellWhite(t *testing.T) {
	imgs := []image.Image{
		capSolidImage(40, 40, color.RGBA{R: 200, A: 255}),
		nil, nil, nil,
	}
	grid := composeGrid(imgs, 60)
	// 第 5 格（中心 150,150）没有图，应为白底
	if c := grid.RGBAAt(150, 150); c.R < 250 || c.G < 250 || c.B < 250 {
		t.Fatalf("缺图的格子应为白色，实际 %v", c)
	}
}

func capSolidImage(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

func capSolidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, capSolidImage(w, h, c)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
