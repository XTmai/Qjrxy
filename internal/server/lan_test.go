// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"net"
	"testing"
)

func TestSortLanURLsPrefersOutbound(t *testing.T) {
	outbound := net.ParseIP("172.20.10.14")
	urls := []string{"http://192.168.137.1:8090", "http://172.20.10.14:8090"}
	sortLanURLs(urls, outbound)
	if urls[0] != "http://172.20.10.14:8090" {
		t.Fatalf("出口 IP 对应地址应排最前，得到 %v", urls)
	}
}

func TestSortLanURLsNoOutboundKeepsOrder(t *testing.T) {
	urls := []string{"http://192.168.1.5:8090", "http://10.0.0.5:8090"}
	sortLanURLs(urls, nil)
	if urls[0] != "http://192.168.1.5:8090" {
		t.Fatalf("无出口 IP 时应保持原序，得到 %v", urls)
	}
}

func TestPortOfAndLoopbackOnly(t *testing.T) {
	cases := []struct {
		addr     string
		port     string
		loopback bool
	}{
		{":8090", "8090", false},
		{"127.0.0.1:8090", "8090", true},
		{"192.168.1.5:8090", "8090", false},
		{"localhost:80", "80", true},
		{"[::1]:9000", "9000", true},
		{"8090", "8090", false},
	}
	for _, c := range cases {
		if got := portOf(c.addr); got != c.port {
			t.Errorf("portOf(%q)=%q 期望 %q", c.addr, got, c.port)
		}
		if got := loopbackOnly(c.addr); got != c.loopback {
			t.Errorf("loopbackOnly(%q)=%v 期望 %v", c.addr, got, c.loopback)
		}
	}
}

func TestIsPrivateIPv4(t *testing.T) {
	yes := []string{"10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.137.1"}
	no := []string{"8.8.8.8", "127.0.0.1", "169.254.1.1", "172.32.0.1", "192.169.0.1"}
	for _, s := range yes {
		if !isPrivateIPv4(net.ParseIP(s).To4()) {
			t.Errorf("%s 应判定为私网", s)
		}
	}
	for _, s := range no {
		if isPrivateIPv4(net.ParseIP(s).To4()) {
			t.Errorf("%s 不应判定为私网", s)
		}
	}
}
