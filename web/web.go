// Package web 内嵌前端静态资源，保证单文件二进制即可部署。
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// FS 返回去掉 dist 前缀后的静态文件系统。
func FS() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}

// assetVersion 是资源指纹，用于给静态文件加版本查询串。
// 二进制内嵌内容不变则指纹不变，前端改动后重新编译会自动变化。
var assetVersion = computeVersion()

func computeVersion() string {
	sub, err := fs.Sub(embedded, "dist")
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

// Version 返回当前静态资源指纹。
func Version() string { return assetVersion }

// Handler 返回静态资源处理器；未知路径回退到 index.html（单页应用）。
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

		// HTML 入口不做缓存，保证资源指纹总是最新
		if path == "index.html" || path == "device.html" {
			w.Header().Set("Cache-Control", "no-cache")
			serveHTML(w, r, sub, path)
			return
		}

		if _, err := fs.Stat(sub, path); err != nil {
			if strings.HasPrefix(r.URL.Path, "/device") {
				w.Header().Set("Cache-Control", "no-cache")
				serveHTML(w, r, sub, "device.html")
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			serveHTML(w, r, sub, "index.html")
			return
		}

		// JS 模块：把相对 import 的路径改写为带版本号，避免浏览器磁盘缓存
		// 留存的旧模块（旧副本没有 ETag，无法协商，只能一直被复用）。
		// 第三方库（vendor/）不做改写，按普通静态文件处理。
		if strings.HasSuffix(path, ".js") && !strings.HasPrefix(path, "vendor/") {
			serveModule(w, r, sub, path)
			return
		}

		// 统一走协商缓存：ES 模块之间互相 import，无法给每个子模块加指纹。
		//
		// 注意：embed.FS 的 ModTime 是零值，http.FileServer 不会设置 Last-Modified，
		// 浏览器拿不到验证器就会一直复用旧副本。这里按内容计算 ETag 补上，
		// 配合 Cache-Control: no-cache 实现「内容变了立刻生效」。
		if strings.HasPrefix(r.URL.Path, "/assets/") || strings.HasPrefix(r.URL.Path, "/vendor/") {
			etag := assetETag(sub, path)
			if etag != "" {
				w.Header().Set("ETag", etag)
				if match := r.Header.Get("If-None-Match"); match != "" && etagInMatch(match, etag) {
					w.Header().Set("Cache-Control", "no-cache")
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}

// moduleNames 是前端拆分的模块文件，用于给 import 路径加版本号。
var moduleNames = []string{
	"api.js", "ui.js", "app.js",
	"page-dashboard.js", "page-accounts.js", "page-tasks.js",
	"page-logs.js", "page-quota.js", "page-security.js",
}

// serveModule 输出 JS 模块，并把相对 import 改写为带版本号的 URL，
// 使每次重新编译后旧模块立即失效。
func serveModule(w http.ResponseWriter, r *http.Request, sub fs.FS, name string) {
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	content := string(data)
	for _, mod := range moduleNames {
		content = strings.ReplaceAll(content,
			"'./"+mod+"'", "'./"+mod+"?v="+assetVersion+"'")
	}

	etag := `"` + assetVersion + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if etagInMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(content))
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

func serveHTML(w http.ResponseWriter, r *http.Request, sub fs.FS, name string) {
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// 给资源引用注入版本号
	content := strings.ReplaceAll(string(data), "/assets/app.css", "/assets/app.css?v="+assetVersion)
	content = strings.ReplaceAll(content, "/assets/app.js", "/assets/app.js?v="+assetVersion)
	content = strings.ReplaceAll(content, "/assets/logo.svg", "/assets/logo.svg?v="+assetVersion)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(content))
}
