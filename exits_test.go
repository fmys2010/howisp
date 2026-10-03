package main

import "testing"

// 出口列表会被界面每隔几秒轮询一次，而隧道状态由健康检查和重连 goroutine
// 在后台改写。这条用例专门盯住这两边的并发：任何一侧绕过 Tunnel 的锁，
// go test -race 就会报出来。
//
// 早先版本正是这里漏了锁——Status/Node/ExitIP 直接裸读裸写，
// 真机上跑 -race 能稳定复现 5 处竞争。
func TestExitsOfConcurrentWithTunnelUpdates(t *testing.T) {
	setPublicIPOverride("1.2.3.4")
	defer setPublicIPOverride("")

	m := NewManager(20, t.TempDir())
	tn := &Tunnel{
		Slot: 1, port: 12345,
		node:   Node{HostName: "vpn-a", CountryCode: "JP", Country: "Japan"},
		status: "up",
	}
	m.mu.Lock()
	m.tunnels[1] = tn
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			// 模拟健康检查判掉线 → 重连 → 重新连上的全过程
			tn.setState("starting", "正在换节点重连")
			tn.setExitIP("")
			tn.setNode(Node{HostName: "vpn-b", CountryCode: "JP", Country: "Japan"})
			tn.setExitIP("203.0.113.9")
			tn.markUp()
		}
	}()

	for i := 0; i < 200; i++ {
		v := m.ExitsOf()
		if len(v.Exits) != 1 {
			t.Fatalf("应当有 1 条出口，实际 %d", len(v.Exits))
		}
	}
	<-done
}

// 落盘同样要和后台改写并发安全：saveState 读的是快照。
func TestSaveStateConcurrentWithTunnelUpdates(t *testing.T) {
	m := NewManager(20, t.TempDir())
	tn := &Tunnel{Slot: 1, port: 12345, node: Node{HostName: "vpn-a"}, status: "up"}
	m.mu.Lock()
	m.tunnels[1] = tn
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			tn.setNode(Node{HostName: "vpn-b", CountryCode: "JP", Config: "cfg"})
			tn.setExitIP("203.0.113.9")
			tn.setState("starting", "重连中")
			tn.markUp()
		}
	}()
	for i := 0; i < 100; i++ {
		if err := m.saveState(); err != nil {
			t.Fatal(err)
		}
	}
	<-done
}