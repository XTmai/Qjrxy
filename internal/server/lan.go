// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"net"
	"strings"
)

// LANBaseURLs 返回本机局域网可访问的基础地址（http://IP:port），
// 供扫码授权页展示与自动打开浏览器使用：手机必须用局域网地址才能
// 访问到本机。
//
// 只收集私网 IPv4（10.x / 172.16-31.x / 192.168.x），不把公网 IP
// 泄漏进二维码；绑定回环地址时返回空（外部本来也访问不到）。
// 排序规则：当前默认路由（真正联网）的网卡排最前——电脑连手机
// 热点/家庭 Wi-Fi 的场景下，这个地址才是手机可达的那个。
func LANBaseURLs(addr string) []string {
	port := portOf(addr)
	if loopbackOnly(addr) {
		return nil
	}
	out := []string{}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !isPrivateIPv4(ip) {
				continue
			}
			out = append(out, "http://"+ip.String()+":"+port)
		}
	}
	sortLanURLs(out, outboundIP())
	return out
}

// outboundIP 探测默认路由出口 IP（UDP dial 不真正发包，只查路由表）。
// 离线或探测失败返回 nil。
func outboundIP() net.IP {
	conn, err := net.Dial("udp", "223.5.5.5:53") // AliDNS：国内可达；UDP 不会发出数据
	if err != nil {
		return nil
	}
	defer conn.Close()
	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

// sortLanURLs 把出口 IP 对应的地址排到最前（纯函数，便于单测）。
func sortLanURLs(urls []string, outbound net.IP) {
	if outbound == nil || len(urls) < 2 {
		return
	}
	want := "http://" + outbound.String() + ":"
	for i, u := range urls {
		if strings.HasPrefix(u, want) && i != 0 {
			urls[0], urls[i] = urls[i], urls[0]
			return
		}
	}
}

// portOf 从监听地址提取端口，缺省 8090。兼容 [::1]:9000 括号形式。
func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	return "8090"
}

// loopbackOnly 判断监听地址是否只绑定回环（127.x / ::1 / localhost）。
// 无法解析的形式（含纯端口 ":8090"）按监听全部网卡处理。
func loopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.ToLower(host) == "localhost"
}

// isPrivateIPv4 判断是否 RFC1918 私网地址。
func isPrivateIPv4(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	b := ip[0]
	switch {
	case b == 10:
		return true
	case b == 172:
		return ip[1] >= 16 && ip[1] <= 31
	case b == 192:
		return ip[1] == 168
	default:
		return false
	}
}
