// 一次性验证工具：模拟青衿的「扫码授权」页面。
//
// 目的：确认今日校园 App 注入 WebView 的 JS Bridge 暴露了哪些能力
// （cpdaily.getTenantID / getTenantInfo / getUserInfo、device.getInfo 等），
// 以及返回的数据结构（是否含 sessionToken / 设备标识）。
//
// 用法：go run ./cmd/qrauth → 手机浏览器打开打印出的局域网地址 → 用
// 今日校园 App 扫页面上的二维码 → 页面在 App 内打开并回传数据。
// 验证完删除本目录。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

var (
	mu       sync.Mutex
	received = map[string]json.RawMessage{}
	token    = fmt.Sprintf("%x", time.Now().UnixNano())
)

const authPage = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1">
<title>Qjrxy · 扫码授权</title>
<style>body{font-family:-apple-system,sans-serif;margin:0;padding:28px 18px;background:#f5f6f8;color:#111}
.box{max-width:460px;margin:0 auto;background:#fff;border-radius:16px;padding:22px;box-shadow:0 8px 30px rgba(0,0,0,.06)}
h1{font-size:17px;margin:0 0 8px}p{font-size:13px;color:#666;line-height:1.6}
.ok{color:#0a7f4f}.err{color:#b3261e}
pre{background:#f4f4f6;border-radius:8px;padding:10px;font-size:11px;overflow:auto;max-height:52vh;white-space:pre-wrap}</style></head>
<body><div class="box"><h1>Qjrxy · 扫码授权</h1><p id="status">初始化…</p><pre id="log"></pre></div>
<script>
(async function () {
  const out = { ts: new Date().toISOString(), ua: navigator.userAgent, href: location.href, steps: [] };
  const statusEl = document.getElementById("status"), logEl = document.getElementById("log");
  const step = (k, v) => { out.steps.push([k, v]); logEl.textContent = JSON.stringify(out.steps, null, 2); };
  const sleep = ms => new Promise(r => setTimeout(r, ms));

  function execAsync(service, action, args) {
    return new Promise(resolve => {
      let done = false;
      const fin = v => { if (!done) { done = true; clearTimeout(t); resolve(v); } };
      const t = setTimeout(() => fin({ timeout: true }), 2000);
      try {
        if (!window.cordova || !window.cordova.exec) return fin({ noCordova: true });
        window.cordova.exec(v => fin({ ok: v }), e => fin({ error: e }), service, action, args || []);
      } catch (e) { fin({ throw: String(e) }); }
    });
  }

  // 1) 加载青衿同款 cordova（CDN）
  const s1 = document.createElement("script");
  s1.src = "https://static.campushoy.com/innerjs/cordova_ios.js";
  document.head.appendChild(s1);
  for (let i = 0; i < 40; i++) { if (window.cordova && window.cordova.exec) break; await sleep(100); }
  step("cordova", typeof window.cordova);

  // 2) 等 deviceready
  if (!window.cordova) { step("noCordova", true); }
  await Promise.race([new Promise(r => document.addEventListener("deviceready", r, { once: true })), sleep(2500)]);
  step("deviceready", true);

  // 3) 采样桥对象
  step("mampKeys", window.mamp ? Object.keys(window.mamp) : null);
  step("mampHttpKeys", window.mamp && window.mamp.http ? Object.keys(window.mamp.http) : null);
  step("bhKeys", window.BH_MOBILE ? Object.keys(window.BH_MOBILE) : (window.BH_MOBILE_SDK ? Object.keys(window.BH_MOBILE_SDK) : null));

  // 4) 设备信息（青衿同款两个插件都试）
  out.deviceA = await execAsync("MAMPDevice", "getDeviceInfo", []);
  step("deviceA(MAMPDevice)", out.deviceA);
  out.deviceB = await execAsync("Device", "getDeviceInfo", []);
  step("deviceB(Device)", out.deviceB);

  // 5) 关键：用 App 原生 HTTP 调 setCookieByToken（青衿同款，GET 无参数）
  const http = (window.mamp && window.mamp.http && window.mamp.http.sendGetRequest) ? window.mamp.http
             : (window.BH_MOBILE_SDK && window.BH_MOBILE_SDK.http && window.BH_MOBILE_SDK.http.sendGetRequest) ? window.BH_MOBILE_SDK.http
             : null;
  step("nativeHttp", !!http);
  if (http) {
    const got = await new Promise(resolve => {
      let done = false;
      const fin = v => { if (!done) { done = true; clearTimeout(t); resolve(v); } };
      const t = setTimeout(() => fin({ timeout: true }), 5000);
      try {
        const r = http.sendGetRequest("https://api.campushoy.com/connect/oauth2/setCookieByToken", fin);
        if (r && typeof r.then === "function") r.then(fin, e => fin({ reject: String(e) }));
      } catch (e) { fin({ throw: String(e) }); }
    });
    out.setCookieByToken = got;
    step("setCookieByToken", got);
  }

  // 6) 回传
  statusEl.textContent = "回传中…";
  try {
    const r = await fetch("/submit?t=__TOKEN__", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(out) });
    const j = await r.json();
    statusEl.textContent = j.ok ? "✅ 完成，可关闭页面" : "回传失败";
    statusEl.className = j.ok ? "ok" : "err";
  } catch (e) { statusEl.textContent = "回传失败：" + e; statusEl.className = "err"; }
})();
</script></body></html>`


func main() {
	addr := "0.0.0.0:18100"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	port := "18100"
	if _, p, err := net.SplitHostPort(addr); err == nil {
		port = p
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		authURL := schemeOf(r) + "://" + r.Host + "/auth?t=" + token
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Qjrxy 扫码测试</title>
<style>body{font-family:-apple-system,sans-serif;background:#f5f6f8;margin:0;padding:28px 16px;color:#111}
.box{max-width:460px;margin:0 auto;background:#fff;border-radius:16px;padding:22px;text-align:center;box-shadow:0 8px 30px rgba(0,0,0,.06)}
img{width:260px;height:260px}p{font-size:13px;color:#666;line-height:1.6;word-break:break-all}
code{background:#f2f3f5;padding:2px 6px;border-radius:6px;font-size:12px}</style></head><body>
<div class="box"><h2 style="font-size:17px;margin:0 0 6px">用今日校园 App 扫码</h2>
<p>App 内打开授权页后会读取 bridge 数据并回传。</p>
<img src="/qr.png?t=%s" alt="QR">
<p>授权页地址：<br><code>%s</code></p>
<p>也可用手机浏览器直接打开上面的地址（不会拿到 bridge 数据，用于验证网络）。</p></div></body></html>`, token, authURL)
	})

	http.HandleFunc("/qr.png", func(w http.ResponseWriter, r *http.Request) {
		authURL := schemeOf(r) + "://" + r.Host + "/auth?t=" + token
		png, _ := qrcode.Encode(authURL, qrcode.Medium, 512)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})

	http.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != token {
			http.Error(w, "token 无效", http.StatusGone)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page := authPage
		// 注入真实 token
		page = replaceAll(page, "__TOKEN__", token)
		_, _ = w.Write([]byte(page))
	})

	http.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != token {
			http.Error(w, "token 无效", http.StatusGone)
			return
		}
		var body json.RawMessage
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		mu.Lock()
		received["payload"] = body
		mu.Unlock()
		// 同时落盘，便于事后查看
		_ = os.WriteFile("qrauth-result.json", body, 0o600)
		log.Printf("收到扫码数据 %d 字节", len(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	http.HandleFunc("/result", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if received["payload"] == nil {
			_, _ = w.Write([]byte(`{"ok":false,"message":"还没有收到数据"}`))
			return
		}
		_, _ = w.Write(received["payload"])
	})

	lan := lanIP()
	log.Printf("引导页: http://%s:%s/", lan, port)
	log.Printf("授权页: http://%s:%s/auth?t=%s", lan, port, token)
	log.Printf("结果:   http://%s:%s/result", lan, port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// schemeOf 根据反向代理头判断外部协议（https 隧道场景）。
func schemeOf(r *http.Request) string {
    if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
        return p
    }
    if r.TLS != nil {
        return "https"
    }
    // 内网穿透域名：外部一定是 https，内部转发通常不带 X-Forwarded-Proto
    if strings.HasSuffix(r.Host, ".lhr.life") {
        return "https"
    }
    return "http"
}

// lanIP 返回本机一个非回环 IPv4（优先默认路由出口）。
func lanIP() string {
	if c, err := net.Dial("udp", "223.5.5.5:80"); err == nil {
		defer c.Close()
		if addr, ok := c.LocalAddr().(*net.UDPAddr); ok && addr.IP != nil {
			return addr.IP.String()
		}
	}
	ifaces, _ := net.Interfaces()
	for _, it := range ifaces {
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func replaceAll(s, old, new string) string {
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
