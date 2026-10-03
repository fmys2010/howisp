package main

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	healthInterval = 10 * time.Second
	healthFailures = 2 // 连续失败几次才判定掉线，避免网络抖动误杀
	healthTimeout  = 6 * time.Second
)

// WatchHealth 周期检查每条隧道是否还能出网，掉线的自动换节点重连。
// VPN Gate 是志愿者节点，运行中掉线很常见。
func (m *Manager) WatchHealth() {
	fails := map[int]int{}

	for range time.Tick(healthInterval) {
		for _, t := range m.Tunnels() {
			if t.statusOf() != "up" {
				continue
			}
			if m.tunnelHealthy(t) {
				fails[t.Slot] = 0
				continue
			}

			fails[t.Slot]++
			if fails[t.Slot] < healthFailures {
				log.Printf("隧道 %d (%s) 探测失败 %d 次", t.Slot, t.nodeOf().HostName, fails[t.Slot])
				continue
			}

			log.Printf("隧道 %d (%s) 已掉线，正在换节点重连", t.Slot, t.nodeOf().HostName)
			fails[t.Slot] = 0
			m.reconnect(t)
		}
	}
}

// tunnelHealthy 判断隧道是否还真的走在 VPN 上。
//
// 只看"能不能出网"是不够的：netns 通过 veth 走母机 NAT，
// openvpn 死掉后照样能出网，只是出口变回了母机 IP。
// 所以要比对出口 IP 是否仍是建立隧道时拿到的那个。
func (m *Manager) tunnelHealthy(t *Tunnel) bool {
	out, err := cmdOutput(exec.Command("ip", "netns", "exec", t.nsName(),
		"curl", "-s", "--max-time", strconv.Itoa(int(healthTimeout.Seconds())),
		"http://api.ipify.org"))
	if err != nil {
		return false
	}
	got := strings.TrimSpace(string(out))
	if got == "" {
		return false
	}
	// 出口 IP 变了说明 VPN 已经断开，流量退回了母机
	return got == t.exitIPOf()
}

// reconnect 就地把一条隧道换到别的节点上，保持槽位与端口不变，
// 这样已经分发出去的客户端配置仍然可用。
func (m *Manager) reconnect(t *Tunnel) {
	t.setState("starting", "正在换节点重连")
	t.setExitIP("")

	// 先杀进程再拆 netns。顺序反了的话，openvpn 会继续活在已被删除的
	// 命名空间里——只要有进程引用，那个 netns 就不会真正释放，
	// 变成谁也看不见、谁也管不着的僵尸。
	t.killOpenVPN()
	t.teardownNetns()

	// 重连是持久化行为：一轮候选全失败不放弃，退避后刷新节点列表再来
	go m.bringUpPersist(t, true)
}