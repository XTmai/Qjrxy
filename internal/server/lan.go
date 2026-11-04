package server

import (
	"net"
	"strings"
)

// LANBaseURLs 返回本机局域网可访问的基础地址（http://IP:port），
// 供扫码授权页展示：手机必须用局域网地址才能访问到本机。
//
// 只收集私网 IPv4（10.x / 172.16-31.x / 192.168.x），不把公网 IP
// 泄漏进二维码；绑定回环地址时返回空（外部本来也访问不到）。
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
	return out
}

// portOf 从监听地址提取端口，缺省 8090。
func portOf(addr string) string {
	if _, port, ok := strings.Cut(addr, ":"); ok && port != "" {
		return port
	}
	return "8090"
}

// loopbackOnly 判断监听地址是否只绑定回环（127.x / ::1 / localhost）。
func loopbackOnly(addr string) bool {
	host, _, ok := strings.Cut(addr, ":")
	if !ok {
		return false // 纯端口形式 ":8090" 监听全部网卡
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	h := strings.ToLower(host)
	return h == "localhost"
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
