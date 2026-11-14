// Package web 内嵌前端静态资源，保证单文件二进制即可部署。
//
// dist 目录由 web/frontend 的 Vite 构建产出（make build）：
// 带 hash 的 assets/* 长缓存，index.html 与 device.html 不缓存。
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:dist
var embedded embed.FS

// FS 返回去掉 dist 前缀后的静态文件系统。
func FS() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}

// Version 返回当前静态资源指纹（诊断用）。
func Version() string { return computeVersion() }

func computeVersion() string {
	sub, err := FS()
	if err != nil {
		return "0"
	}
	hash := sha256.New()
	_ = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return nil
		}
		hash.Write([]byte(path))
		hash.Write(data)
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))[:12]
}

// Handler 返回静态资源处理器；未知路径回退到 index.html（单页应用），
// /device 前缀回退到 device.html（今日校园 App 内的扫码授权页）。
func Handler() (http.Handler, error) {
	sub, err := FS()
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// HTML 入口不做缓存，保证引用的资源指纹总是最新
		if path == "index.html" || path == "device.html" || path == "scan-login.html" {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, path, time.Time{}, strings.NewReader(mustRead(sub, path)))
			return
		}

		if _, err := fs.Stat(sub, path); err != nil {
			// SPA / device / scan-login 页回退
			fallback := "index.html"
			if strings.HasPrefix(r.URL.Path, "/device") {
				fallback = "device.html"
			} else if strings.HasPrefix(r.URL.Path, "/scan-login") {
				fallback = "scan-login.html"
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, fallback, time.Time{}, strings.NewReader(mustRead(sub, fallback)))
			return
		}

		// Vite 产物：文件名带内容 hash，直接长缓存
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			fileServer.ServeHTTP(w, r)
			return
		}

		// 其余静态文件（favicon.svg 等）：按内容 ETag 协商。
		// embed.FS 的 ModTime 是零值，http.FileServer 不会设置 Last-Modified。
		if etag := assetETag(sub, path); etag != "" {
			w.Header().Set("ETag", etag)
			if match := r.Header.Get("If-None-Match"); match != "" && etagInMatch(match, etag) {
				w.Header().Set("Cache-Control", "no-cache")
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}

func mustRead(sub fs.FS, name string) string {
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		return ""
	}
	return string(data)
}

// assetETag 按文件内容计算强 ETag，形如 "3f2a1b..."。
func assetETag(sub fs.FS, name string) string {
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:])[:16] + `"`
}

// etagInMatch 判断 If-None-Match 头是否命中当前 ETag（支持逗号分隔与通配）。
func etagInMatch(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == etag || part == "*" {
			return true
		}
		// 弱验证器前缀
		if strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}
