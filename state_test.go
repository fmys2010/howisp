package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 状态要能落盘再读回来：端口与凭据是用户已经分发出去的东西，
// 重启后必须原样恢复，否则所有客户端配置一次性失效。
func TestSaveStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(20, dir)
	tn := &Tunnel{
		Slot: 1, port: 12345,
		node:   Node{HostName: "vpn-a", CountryCode: "JP", Config: "cfg"},
		status: "starting",
		cred:   SocksCred{User: "u", Pass: "p"},
	}
	m.tunnels[1] = tn

	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st persistedState
	if err := json.Unmarshal(blob, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Tunnels) != 1 {
		t.Fatalf("应当存下 1 条，实际 %d", len(st.Tunnels))
	}
	got := st.Tunnels[0]
	if got.Port != 12345 || got.HostName != "vpn-a" || got.Config != "cfg" {
		t.Fatalf("落盘内容不对: %+v", got)
	}
	if got.SocksUser != "u" || got.SocksPass != "p" {
		t.Fatalf("凭据没存下来: %+v", got)
	}
}

// 用户主动停掉的隧道不该留在状态里，否则重启会把它拉回来。
func TestSaveStateSkipsStopped(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(20, dir)
	m.tunnels[1] = &Tunnel{Slot: 1, port: 1, status: "up"}
	m.tunnels[2] = &Tunnel{Slot: 2, port: 2, status: "stopped"}

	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	blob, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	var st persistedState
	if err := json.Unmarshal(blob, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Tunnels) != 1 || st.Tunnels[0].Slot != 1 {
		t.Fatalf("只该存下未停的那条，实际 %+v", st.Tunnels)
	}
}

// 恢复时端口与节点要读回来，老状态文件缺凭据字段时要自动补一套。
func TestRestoreStateKeepsPortAndNode(t *testing.T) {
	dir := t.TempDir()
	blob := `{"tunnels":[{"slot":1,"port":12345,"hostname":"vpn-a","country_code":"JP","config":"x"}]}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(blob), 0600); err != nil {
		t.Fatal(err)
	}

	m := NewManager(20, dir)
	n, err := m.restoreState()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应当恢复 1 条，实际 %d", n)
	}
	m.mu.RLock()
	tn := m.tunnels[1]
	m.mu.RUnlock()
	if tn == nil {
		t.Fatal("隧道没恢复出来")
	}
	v := tn.snapshot()
	if v.Port != 12345 || v.Node.HostName != "vpn-a" || v.Node.Config != "x" {
		t.Fatalf("恢复内容不对: %+v", v)
	}
	if v.Cred.User == "" || v.Cred.Pass == "" {
		t.Fatalf("老格式应当补一套凭据，实际 %+v", v.Cred)
	}
	// 收尾：别让后台的 bringUpPersist 继续折腾
	tn.setStatus("stopped")
}