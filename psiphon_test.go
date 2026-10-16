package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 就绪判断全靠通知流：端口、隧道数、可用地区、出错原因都要读对。
// 隧道数取最后一条——psiphon 掉线重连时会先报 0 再报 1。
func TestParsePsiphonNotices(t *testing.T) {
	long := `{"noticeType":"Info","data":{"message":"` + strings.Repeat("x", 200<<10) + `"}}`
	stream := strings.Join([]string{
		`{"noticeType":"ListeningSocksProxyPort","data":{"port":41234},"timestamp":"2026-01-01T00:00:00Z"}`,
		`不是 JSON`,
		long,
		`{"noticeType":"Tunnels","data":{"count":1}}`,
		`{"noticeType":"Error","data":{"message":"dial failed"}}`,
		// 连上后必报的 DSL 拉取失败不算原因，不能盖掉前面真正的错误
		`{"noticeType":"Error","data":{"message":"tunneled DSL fetch failed: psiphon.runTunneledDSLFetcher#153: missing public key"}}`,
		`{"noticeType":"AvailableEgressRegions","data":{"regions":["US","JP"]}}`,
		`{"noticeType":"Tunnels","data":{"count":0}}`,
		`{"noticeType":"Tunnels","data":{"count":1}}`,
	}, "\n")
	st := parsePsiphonNotices(strings.NewReader(stream))
	if st.socksPort != 41234 {
		t.Fatalf("端口读错: %d", st.socksPort)
	}
	if st.tunnels != 1 {
		t.Fatalf("隧道数应取最后一条，实际 %d", st.tunnels)
	}
	if strings.Join(st.regions, ",") != "US,JP" {
		t.Fatalf("地区读错: %v", st.regions)
	}
	if st.lastErr != "dial failed" {
		t.Fatalf("错误原因读错: %q", st.lastErr)
	}
	if st.portInUse {
		t.Fatal("没有 SocksProxyPortInUse 通知，不该判端口被占")
	}
}

// 配置要让 psiphon 自己挑端口、关掉 HTTP 代理；不限地区时不写 EgressRegion，交给 psiphon 挑。
func TestRenderPsiphonConfig(t *testing.T) {
	blob, err := renderPsiphonConfig("JP")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(blob, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["EgressRegion"] != "JP" {
		t.Fatalf("地区没写进去: %v", cfg["EgressRegion"])
	}
	if cfg["LocalSocksProxyPort"] != float64(0) || cfg["DisableLocalHTTPProxy"] != true {
		t.Fatalf("端口与 HTTP 代理设置不对: %v", cfg)
	}
	for _, k := range []string{"PropagationChannelId", "SponsorId", "RemoteServerListUrl", "RemoteServerListSignaturePublicKey"} {
		if s, _ := cfg[k].(string); s == "" {
			t.Fatalf("缺少接入参数 %s", k)
		}
	}

	blob, _ = renderPsiphonConfig("")
	if strings.Contains(string(blob), "EgressRegion") {
		t.Fatal("不限地区时不该写 EgressRegion")
	}
}

// startSocksUpstream 起一个本地 SOCKS5 服务，冒充 psiphon 的本地端口。
// 复用本项目自己的混合端口实现：无凭据、直接拨号。
func startSocksUpstream(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveMixed(c, nil, func(network, addr string) (net.Conn, error) {
				return net.DialTimeout(network, addr, 3*time.Second)
			}, true)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// startEcho 起一个回显服务，返回它的端口。
func startEcho(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// 经 psiphon 本地端口出站：IP 和域名两种目标都要能连通，数据原样往返。
func TestPsiphonDialThroughLocalSocks(t *testing.T) {
	up := startSocksUpstream(t)
	echo := startEcho(t)
	tn := &Tunnel{node: psiphonNode("US"), psiPort: up}

	for _, target := range []string{
		net.JoinHostPort("127.0.0.1", strconv.Itoa(echo)),
		net.JoinHostPort("localhost", strconv.Itoa(echo)),
	} {
		conn, err := tn.psiphonDial("tcp", target)
		if err != nil {
			t.Fatalf("连 %s 失败: %v", target, err)
		}
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write([]byte("ping\n")); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(conn).ReadString('\n')
		conn.Close()
		if err != nil || line != "ping\n" {
			t.Fatalf("经 %s 回显不对: %q %v", target, line, err)
		}
	}
}

// 上游拒绝时要把失败带出来，而不是返回一条半通的连接。
func TestPsiphonDialReportsUpstreamFailure(t *testing.T) {
	up := startSocksUpstream(t)
	// 先占一个端口再关掉，拿到一个大概率没人监听的端口
	ln, _ := net.Listen("tcp4", "127.0.0.1:0")
	dead := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	tn := &Tunnel{node: psiphonNode(""), psiPort: up}
	if conn, err := tn.psiphonDial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(dead))); err == nil {
		conn.Close()
		t.Fatal("目标没人监听，应当报错")
	}
}

// 没连上时不能去碰端口 0；UDP 一律拒绝。
func TestPsiphonDialGuards(t *testing.T) {
	tn := &Tunnel{node: psiphonNode("US")}
	if _, err := tn.psiphonDial("tcp", "example.com:80"); err == nil {
		t.Fatal("psiphon 还没连上，应当报错")
	}
	tn.psiPort = 1
	if _, err := tn.psiphonDial("udp", "8.8.8.8:53"); !errors.Is(err, errUDPUnsupported) {
		t.Fatalf("UDP 应当直接拒绝，实际 %v", err)
	}
}

// Psiphon 出口不转 UDP：UDP ASSOCIATE 要明确回"不支持"，
// 不能答应下来再把包全丢掉，让客户端干等超时。
func TestSocksUDPAssociateRejectedWithoutUDP(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go serveMixed(server, nil, func(network, addr string) (net.Conn, error) {
		return nil, io.EOF
	}, false)
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))

	go func() { _, _ = client.Write([]byte{socksVer5, 1, authNone}) }()
	sel := make([]byte, 2)
	if _, err := io.ReadFull(client, sel); err != nil || sel[1] != authNone {
		t.Fatalf("方法协商失败: %v %v", sel, err)
	}
	go func() {
		_, _ = client.Write([]byte{socksVer5, cmdUDPAssociate, 0, atypIPv4, 0, 0, 0, 0, 0, 0})
	}()
	rep := make([]byte, 4)
	if _, err := io.ReadFull(client, rep); err != nil {
		t.Fatalf("读应答失败: %v", err)
	}
	if rep[1] != repCmdNotSupp {
		t.Fatalf("应当回命令不支持(%d)，实际 %d", repCmdNotSupp, rep[1])
	}
}

// Psiphon 不挑节点，同地区开几条都行，唯一的约束是槽位。
func TestPickPsiphon(t *testing.T) {
	m := NewManager(5, t.TempDir())
	m.tunnels[1] = &Tunnel{Slot: 1, node: psiphonNode("US"), status: "up"}

	got, err := m.pickPsiphon("us", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("同地区应当能开 3 条，实际 %d", len(got))
	}
	for _, n := range got {
		if !n.isPsiphon() || n.CountryCode != "US" {
			t.Fatalf("挑出来的不对: %+v", n)
		}
	}

	// 只剩 4 个槽位，按地区顺序截断
	got, err = m.pickPsiphon("", 2, true)
	if err != nil {
		t.Fatal(err)
	}
	regions := psiphonRegions()
	if len(got) != 4 || got[0].CountryCode != regions[0] || got[2].CountryCode != regions[1] {
		t.Fatalf("每个国家应按顺序截断到 4 条，实际 %+v", got)
	}

	if _, err := m.pickPsiphon("USA", 1, false); err == nil {
		t.Fatal("三个字母的地区码应当报错")
	}

	for i := 2; i <= 5; i++ {
		m.tunnels[i] = &Tunnel{Slot: i, status: "up"}
	}
	if _, err := m.pickPsiphon("US", 1, false); err == nil {
		t.Fatal("槽位满了应当报错")
	}
}

// 向导里每个地区的"可用数"都是剩余槽位。
func TestPsiphonRegionsShowsRoom(t *testing.T) {
	m := NewManager(3, t.TempDir())
	m.tunnels[1] = &Tunnel{Slot: 1, status: "up"}
	stats := m.PsiphonRegions()
	if len(stats) != len(psiphonRegions()) {
		t.Fatalf("地区数不对: %d", len(stats))
	}
	for _, s := range stats {
		if s.Available != 2 {
			t.Fatalf("%s 的可用数应为剩余槽位 2，实际 %d", s.Code, s.Available)
		}
	}
}

// 实时清单要过滤掉不像国家码的东西；空清单不能把列表清空。
func TestSetPsiphonRegions(t *testing.T) {
	prev := psiphonRegions()
	t.Cleanup(func() { setPsiphonRegions(prev) })

	setPsiphonRegions(nil)
	if len(psiphonRegions()) != len(prev) {
		t.Fatal("空清单不该替换现有列表")
	}
	setPsiphonRegions([]string{"jp", "US", "XXX", "", "US", "1A"})
	if got := strings.Join(psiphonRegions(), ","); got != "JP,US" {
		t.Fatalf("过滤或排序不对: %s", got)
	}
}

// Psiphon 出口的来源和地区要存盘，重启后还原成同一地区的 Psiphon 出口，
// 不能被当成一个已下线的 VPN Gate 节点。
func TestPersistPsiphonTunnel(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(20, dir)
	m.tunnels[3] = &Tunnel{
		Slot: 3, port: 23456, node: psiphonNode("SG"), status: "up",
		cred: SocksCred{User: "u", Pass: "p"},
	}
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	blob, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	var st persistedState
	if err := json.Unmarshal(blob, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Tunnels) != 1 || st.Tunnels[0].Provider != providerPsiphon || st.Tunnels[0].CountryCode != "SG" {
		t.Fatalf("落盘内容不对: %+v", st.Tunnels)
	}

	// 就算 VPN Gate 列表里碰巧有同名主机，也要还原成 Psiphon
	known := map[string]Node{"psiphon-sg": {HostName: "psiphon-sg", Config: "ovpn"}}
	n := st.Tunnels[0].node(known)
	if !n.isPsiphon() || n.CountryCode != "SG" || n.Config != "" {
		t.Fatalf("还原出来的节点不对: %+v", n)
	}

	// 老状态文件没有 provider 字段，照旧当 VPN Gate
	old := persistedTunnel{HostName: "vpn-a", CountryCode: "JP", Config: "x"}
	if n := old.node(nil); n.isPsiphon() || n.HostName != "vpn-a" || n.Config != "x" {
		t.Fatalf("老格式还原不对: %+v", n)
	}
}

// 出口列表要标出来源；Psiphon 不限地区时给一个看得懂的名字。
func TestExitsOfMarksPsiphon(t *testing.T) {
	setPublicIPOverride("1.2.3.4")
	defer setPublicIPOverride("")

	m := NewManager(20, t.TempDir())
	m.tunnels[1] = &Tunnel{Slot: 1, node: psiphonNode(""), status: "up"}
	m.tunnels[2] = &Tunnel{Slot: 2, node: Node{HostName: "vpn-a", CountryCode: "JP"}, status: "up"}
	v := m.ExitsOf()
	if v.Exits[0].Provider != providerPsiphon || v.Exits[0].Country != "不限地区" {
		t.Fatalf("Psiphon 出口显示不对: %+v", v.Exits[0])
	}
	if v.Exits[1].Provider != "vpngate" {
		t.Fatalf("VPN Gate 出口来源不对: %+v", v.Exits[1])
	}
}

// 本地已有校验通过的文件就直接用，不去下载；校验不过的不能用。
func TestPsiphonBinaryUsesVerifiedLocalCopy(t *testing.T) {
	t.Setenv("FANOUT_PSIPHON_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin", "psiphon-tunnel-core")
	if err := os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("fake psiphon"), 0755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("fake psiphon"))

	prevSum, prevURL := psiphonBinSum, psiphonBinURL
	psiphonBinMu.Lock()
	prevPath := psiphonBinPath
	psiphonBinPath = ""
	psiphonBinMu.Unlock()
	t.Cleanup(func() {
		psiphonBinSum, psiphonBinURL = prevSum, prevURL
		psiphonBinMu.Lock()
		psiphonBinPath = prevPath
		psiphonBinMu.Unlock()
	})
	// 万一走到下载，连一个必然失败的地址，别真的去 GitHub
	psiphonBinURL = "http://127.0.0.1:1/psiphon"

	psiphonBinSum = hex.EncodeToString(sum[:])
	got, err := psiphonBinary(dir)
	if err != nil || got != bin {
		t.Fatalf("应当直接用本地文件，实际 %q %v", got, err)
	}

	psiphonBinMu.Lock()
	psiphonBinPath = ""
	psiphonBinMu.Unlock()
	psiphonBinSum = strings.Repeat("0", 64)
	if _, err := psiphonBinary(dir); err == nil {
		t.Fatal("校验和不符又下载不到，应当报错")
	}
}

// 环境变量指定的文件优先，不存在要说清楚。
func TestPsiphonBinaryEnvOverride(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "my-psiphon")
	if err := os.WriteFile(custom, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FANOUT_PSIPHON_BIN", custom)
	if got, err := psiphonBinary(dir); err != nil || got != custom {
		t.Fatalf("应当用环境变量指定的文件，实际 %q %v", got, err)
	}
	t.Setenv("FANOUT_PSIPHON_BIN", filepath.Join(dir, "missing"))
	if _, err := psiphonBinary(dir); err == nil {
		t.Fatal("指定的文件不存在，应当报错")
	}
}

func TestArgvHasFlagValue(t *testing.T) {
	argv := []string{"/x/psiphon", "-config", "/a/config.json", "-dataRootDirectory", "/a/data"}
	if !argvHasFlagValue(argv, "-dataRootDirectory", "/a/data") {
		t.Fatal("应当认出来")
	}
	if argvHasFlagValue(argv, "-dataRootDirectory", "/b/data") {
		t.Fatal("别的目录不该认")
	}
}
