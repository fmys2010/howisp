package main

import (
	"strings"
	"testing"
)

func TestParseWarpStatus(t *testing.T) {
	cases := map[string]string{
		"Status update: Connected":                                  "connected",
		"Status update: Connected\nSuccess":                         "connected",
		"Status update: Connecting":                                 "connecting",
		"Status update: Disconnected. Reason: Manual Disconnection": "disconnected",
		"Status update: Disconnected. Reason: Registration Missing": "disconnected",
		"": "unknown",
		"Unable to connect to CloudflareWARP daemon": "unknown",
	}
	for in, want := range cases {
		if got := parseWarpStatus(in); got != want {
			t.Errorf("parseWarpStatus(%q) = %q，应为 %q", in, got, want)
		}
	}
}

func TestWarpUsageError(t *testing.T) {
	if !warpUsageError("error: unrecognized subcommand 'registration'") {
		t.Fatal("老版本不认 registration 子命令，应当识别成用法错误")
	}
	if !warpUsageError("error: unexpected argument 'new' found") {
		t.Fatal("多余参数也是用法错误")
	}
	if warpUsageError("Error: Missing registration") {
		t.Fatal("没注册不是用法错误，不该退回老写法")
	}
}

func TestWarpInstallScript(t *testing.T) {
	s, err := warpInstallScript(map[string]string{"ID": "ubuntu", "ID_LIKE": "debian", "VERSION_CODENAME": "jammy"})
	if err != nil || !strings.Contains(s, "pkg.cloudflareclient.com/ jammy main") || !strings.Contains(s, "install -y -qq cloudflare-warp") {
		t.Fatalf("Ubuntu 的脚本不对: %v\n%s", err, s)
	}
	if _, err := warpInstallScript(map[string]string{"ID": "debian"}); err == nil {
		t.Fatal("读不到代号时应当报错，不能拼出 ' main' 这种坏的软件源")
	}
	s, err = warpInstallScript(map[string]string{"ID": "rocky", "ID_LIKE": "rhel centos fedora"})
	if err != nil || !strings.Contains(s, "cloudflare-warp-ascii.repo") {
		t.Fatalf("RHEL 系的脚本不对: %v\n%s", err, s)
	}
	if _, err := warpInstallScript(map[string]string{"ID": "alpine"}); err == nil {
		t.Fatal("Alpine 没有官方包，应当报错并让人手动装")
	}
}

// WARP 出口最多一个：pickWarp 和 Start 两处都要拦。
func TestWarpSingleton(t *testing.T) {
	m := NewManager(5, t.TempDir())

	got, err := m.pickWarp(1, false)
	if err != nil || len(got) != 1 || !got[0].isWarp() {
		t.Fatalf("没有 WARP 出口时应当能挑出一个: %+v %v", got, err)
	}
	if _, err := m.pickWarp(2, false); err == nil {
		t.Fatal("一次要两个应当报错")
	}
	if _, err := m.pickWarp(1, true); err == nil {
		t.Fatal("「每个国家」对 WARP 没有意义，应当报错")
	}

	m.tunnels[1] = &Tunnel{Slot: 1, node: warpNode(), status: "up"}
	if _, err := m.pickWarp(1, false); err != errWarpExists {
		t.Fatalf("已经有一条时应当拒绝: %v", err)
	}
	if _, err := m.Start(warpNode()); err != errWarpExists {
		t.Fatalf("Start 也要拦住第二条: %v", err)
	}
	if len(m.tunnels) != 1 {
		t.Fatalf("被拒绝的不该占槽位: %d", len(m.tunnels))
	}
}

// 存盘再读回来还是 WARP 出口；界面上也要标成 warp。
func TestWarpPersistAndView(t *testing.T) {
	p := persistedTunnel{Provider: providerWarp, Slot: 2, Port: 1234}
	if n := p.node(nil); !n.isWarp() {
		t.Fatalf("存盘的 WARP 出口读回来变成了 %+v", n)
	}

	m := NewManager(5, t.TempDir())
	m.tunnels[2] = &Tunnel{Slot: 2, port: 1234, node: warpNode(), status: "up"}
	v := m.ExitsOf()
	if len(v.Exits) != 1 || v.Exits[0].Provider != providerWarp || v.Exits[0].Country != warpLabel {
		t.Fatalf("界面数据不对: %+v", v.Exits)
	}
}

// WARP 不转 UDP，和 Psiphon 一样要明确拒绝，不能假装通着。
func TestWarpOutboundTCPOnly(t *testing.T) {
	tun := &Tunnel{Slot: 1, node: warpNode()}
	_, udp := tun.outbound()
	if udp {
		t.Fatal("WARP 出口不该声明支持 UDP")
	}
	if _, err := tun.warpDial("udp", "1.1.1.1:53"); err != errWarpUDP {
		t.Fatalf("UDP 应当被拒: %v", err)
	}
}
