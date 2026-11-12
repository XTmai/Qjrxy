package cpdaily

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// 验证码图片必须不带任何账号凭证：图片在独立 CDN 域上，
// 把 MOD_AUTH_CAS / sessionTokenKey 发过去等于把登录票据交给第三方主机。
func TestCaptchaImageRequestOmitsSessionCookie(t *testing.T) {
	var gotCookie, gotReferer, gotTokenKey, gotSessionToken string
	mux := http.NewServeMux()
	mux.HandleFunc("/img/x.png", func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotReferer = r.Header.Get("Referer")
		gotTokenKey = r.Header.Get("sessionTokenKey")
		gotSessionToken = r.Header.Get("SessionToken")
		_, _ = w.Write(capSolidPNG(t, 8, 8, color.RGBA{A: 255}))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	hc := srv.Client()
	c := NewClient(ClientOptions{HTTPClient: hc}, map[string]string{
		"MOD_AUTH_CAS": "secret-ticket",
		"sessionToken": "secret-session",
	})
	if _, err := c.fetchCaptchaImage(context.Background(), srv.URL+"/img/x.png"); err != nil {
		t.Fatalf("下载图片失败：%v", err)
	}
	if gotCookie != "" {
		t.Fatalf("图片请求不应携带会话 Cookie，实际 %q", gotCookie)
	}
	if gotTokenKey != "" {
		t.Fatalf("图片请求不应携带 sessionTokenKey，实际 %q", gotTokenKey)
	}
	if gotSessionToken != "" {
		t.Fatalf("图片请求不应携带 SessionToken，实际 %q", gotSessionToken)
	}
	if !strings.HasPrefix(gotReferer, srv.URL) {
		t.Fatalf("应带上图片域名自身的 Referer，实际 %q", gotReferer)
	}
}

// 业务接口仍然要带 Cookie——不能因为图片那条改了就把会话弄丢。
func TestBusinessRequestKeepsSessionCookie(t *testing.T) {
	var gotCookie string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		_, _ = io.WriteString(w, `{"code":"0"}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := NewClient(ClientOptions{HTTPClient: srv.Client()}, map[string]string{"MOD_AUTH_CAS": "ticket-1"})
	resp, err := c.do(context.Background(), http.MethodGet, srv.URL+"/api/ping", nil, nil)
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer resp.Body.Close()
	if !strings.Contains(gotCookie, "MOD_AUTH_CAS=ticket-1") {
		t.Fatalf("业务请求应携带会话 Cookie，实际 %q", gotCookie)
	}
}
