package main

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"sync"
	"time"
)

// Manager 维护所有隧道，负责分配槽位与端口。
type Manager struct {
	mu       sync.RWMutex
	tunnels  map[int]*Tunnel
	nodes    []Node
	fetched  time.Time
	workDir  string
	maxSlots int
	jobs     JobStore
}

func NewManager(maxSlots int, workDir string) *Manager {
	return &Manager{
		tunnels:  map[int]*Tunnel{},
		workDir:  workDir,
		maxSlots: maxSlots,
	}
}

// RefreshNodes 重新拉取节点列表。
func (m *Manager) RefreshNodes() (int, error) {
	nodes, err := fetchNodes(60 * time.Second)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	m.nodes = nodes
	m.fetched = time.Now()
	m.mu.Unlock()
	return len(nodes), nil
}

func (m *Manager) Nodes() ([]Node, time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Node, len(m.nodes))
	copy(out, m.nodes)
	return out, m.fetched
}

// Tunnels 返回当前隧道，按槽位排序。
//
// 返回的是指针：隧道的运行态字段是变动的，要读就调 Tunnel 的快照方法，
// 不要直接读字段（那些字段受 Tunnel.mu 保护）。
func (m *Manager) Tunnels() []*Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// Views 返回全部隧道的只读快照，供 HTTP 接口与状态落盘使用。
func (m *Manager) Views() []TunnelView {
	tunnels := m.Tunnels()
	out := make([]TunnelView, 0, len(tunnels))
	for _, t := range tunnels {
		out = append(out, t.snapshot())
	}
	return out
}

// freeSlot 找一个未占用的槽位。槽位同时决定端口与网段。
func (m *Manager) freeSlot() (int, error) {
	for i := 1; i <= m.maxSlots; i++ {
		if _, used := m.tunnels[i]; !used {
			return i, nil
		}
	}
	return 0, fmt.Errorf("槽位已满（上限 %d）", m.maxSlots)
}

// Start 为指定节点开一条隧道，返回分配到的本地端口。
func (m *Manager) Start(node Node) (*Tunnel, error) {
	m.mu.Lock()
	slot, err := m.freeSlot()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	// 端口随机取，避免固定规律撞上机器上的其他服务
	taken := map[int]bool{}
	for _, other := range m.tunnels {
		taken[other.PortNum()] = true
	}
	port, err := freeRandomPort(taken)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	cred, err := newSocksCred()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	t := &Tunnel{
		Slot:   slot,
		port:   port,
		node:   node,
		status: "starting",
		since:  time.Now(),
		cred:   cred,
	}
	m.tunnels[slot] = t
	m.mu.Unlock()

	go m.bringUp(t)
	return t, nil
}

// bringUp 把一条隧道拉起来。手动新建走这条路：一轮候选全失败就标 failed，
// 让用户立刻看到原因并重试。
func (m *Manager) bringUp(t *Tunnel) {
	m.bringUpPersist(t, false)
}

// 自动重连的退避区间：一轮候选全挂后等一会儿再刷新节点列表重来，
// 别把死节点列表打爆，也别让恢复拖太久。
const (
	reconnectBackoffMin = 5 * time.Second
	reconnectBackoffMax = 60 * time.Second
)

// bringUpPersist 把一条隧道拉起来。
//
// persist=false（手动新建）：走一轮候选，全失败就标 failed，让用户能立刻看到并重试。
// persist=true（自动重连 / 重启恢复）：一轮全失败不放弃，退避后刷新节点列表再来一轮，
// 一直循环到连上或这条隧道被用户停掉。VPN Gate 死节点多，"当前都不可用"往往只是
// 这一批候选恰好都挂了，过一会儿就有新节点，不该让出口永久躺死。
func (m *Manager) bringUpPersist(t *Tunnel, persist bool) {
	backoff := reconnectBackoffMin
	for {
		if m.tryCandidates(t) {
			return
		}
		// 隧道已被用户停掉或从管理器移除，别再重试
		if !persist || !m.tunnelActive(t) {
			if persist {
				return
			}
			t.setStatus("failed")
			if serr := m.saveState(); serr != nil {
				log.Printf("保存状态失败: %v", serr)
			}
			return
		}

		// 把最后一次真实失败原因带上：只报"暂无可用节点"用户没法判断
		// 是自己缺依赖、节点全挂还是网络不通。
		t.setState("starting", fmt.Sprintf("暂无可用节点，%.0f 秒后重试（最后错误：%s）",
			backoff.Seconds(), shortErr(t.errOf())))
		log.Printf("隧道 %d 一轮候选均失败，%.0f 秒后刷新节点重试", t.Slot, backoff.Seconds())
		time.Sleep(backoff)
		if !m.tunnelActive(t) {
			return
		}
		if _, err := m.RefreshNodes(); err != nil {
			log.Printf("重试前刷新节点列表失败: %v", err)
		}
		if backoff < reconnectBackoffMax {
			backoff *= 2
			if backoff > reconnectBackoffMax {
				backoff = reconnectBackoffMax
			}
		}
	}
}

// tryCandidates 走一轮候选节点，成功返回 true。
//
// 每个候选失败都写日志并留在 t.Err 上：VPN Gate 的节点失败很常见，
// 早先版本把真实错误丢掉、只显示"已换到第 N 个候选节点"，出问题时
// 从界面到日志都查不出原因。
func (m *Manager) tryCandidates(t *Tunnel) bool {
	candidates := m.candidatesFor(t)
	for i, node := range candidates {
		if !m.tunnelActive(t) {
			return false
		}
		// 其他隧道可能在重试期间占用了这个节点，跳过以免多个端口撞同一出口 IP
		if i > 0 && m.nodeInUse(node.HostName, t.Slot) {
			continue
		}
		t.setNode(node)
		if i > 0 {
			t.setState("starting", fmt.Sprintf("已换到第 %d 个候选节点", i+1))
		} else {
			t.setState("starting", "")
		}

		err := m.tryNode(t)
		if err == nil {
			t.markUp()
			if serr := m.saveState(); serr != nil {
				log.Printf("保存状态失败: %v", serr)
			}
			return true
		}
		log.Printf("隧道 %d 连接节点 %s 失败: %v", t.Slot, node.HostName, err)
		t.setError(shortErr(err.Error()))
		t.teardownNetns()
	}
	return false
}

// tunnelActive 判断这条隧道是否还归管理器所有且未被用户停掉。
// 用指针比对：Stop 会从 map 里删除并把 Status 置 stopped，
// 重连循环据此退出，避免对着一条已经不存在的隧道空转。
func (m *Manager) tunnelActive(t *Tunnel) bool {
	if t.statusOf() == "stopped" {
		return false
	}
	m.mu.RLock()
	cur, ok := m.tunnels[t.Slot]
	m.mu.RUnlock()
	return ok && cur == t
}

// tryNode 尝试用当前节点把隧道拉起来。
func (m *Manager) tryNode(t *Tunnel) error {
	if err := t.setupNetns(); err != nil {
		return err
	}
	if err := t.startOpenVPN(m.workDir); err != nil {
		return err
	}
	if !t.serving() {
		if err := t.serve(); err != nil {
			return err
		}
	}
	ip, err := t.probeExitIP()
	if err != nil {
		return err
	}
	t.setExitIP(ip)
	return nil
}

// candidatesFor 以这条隧道当前的节点打头，后面跟上同地区的其他节点作为备选。
//
// 打头的一定是当前节点：自动重连的目标是把这条出口恢复原样，先试原节点。
// 备选会避开用户手动换掉过的节点——那些是他明确不想要的 IP，
// 让重连悄悄换回去等于撤销了他的操作。
func (m *Manager) candidatesFor(t *Tunnel) []Node {
	const maxTries = 6
	first := t.nodeOf()
	avoid := t.swapAvoid()

	m.mu.RLock()
	defer m.mu.RUnlock()

	used := map[string]bool{first.HostName: true}
	for _, other := range m.tunnels {
		used[other.nodeOf().HostName] = true
	}

	// 地区决定了备选范围，缺失时先从当前列表补一次，
	// 否则会退化成"任意地区都算同区"。
	region := first.CountryCode
	if region == "" {
		for _, n := range m.nodes {
			if n.HostName == first.HostName {
				region = n.CountryCode
				break
			}
		}
	}

	out := []Node{first}
	for _, n := range m.nodePoolLocked() {
		if len(out) >= maxTries {
			break
		}
		if used[n.HostName] || avoid[n.HostName] {
			continue
		}
		// 地区实在拿不到时不做限制，总比连不上强
		if region != "" && n.CountryCode != region {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Stop 停掉一条隧道并释放槽位。
func (m *Manager) Stop(slot int) error {
	m.mu.Lock()
	t, ok := m.tunnels[slot]
	if ok {
		delete(m.tunnels, slot)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}
	t.stop()
	if err := m.saveState(); err != nil {
		log.Printf("保存状态失败: %v", err)
	}
	return nil
}

// Swap 把一条隧道换到同地区的另一个节点上，端口与已分发的客户端配置保持不变。
//
// 与健康检查的自动重连不同：那边优先重连原节点（目标是恢复），
// 这里用户是嫌当前出口 IP 不好用，必须真的换一个。
//
// 换过的节点会记进历史一并避开。只排除"当前这个"是不够的：
// A 换成 B 之后 A 就空出来了，而候选是按速度排的，A 往往又排在最前面，
// 于是再点一次就换回了 A。历史让每次点击都真的换一个没用过的。
func (m *Manager) Swap(slot int) error {
	m.mu.RLock()
	t, ok := m.tunnels[slot]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}
	if t.statusOf() == "starting" {
		return fmt.Errorf("这个出口正在连接中，稍等一下")
	}

	node, err := m.pickSwapTarget(t)
	if err != nil {
		return err
	}
	t.setNode(node)
	m.reconnect(t)
	return nil
}

// pickSwapTarget 给"换节点"挑下一个目标，并把该跳过的节点记进历史。
//
// 记两个：换下来的那个，以及刚挑中的这个。挑中的也记是因为真机上踩到过——
// 它连不上时会被候选列表换成别人，但它自己没进历史，
// 于是下次点换节点又从它开始试一遍，白等一轮握手超时。
func (m *Manager) pickSwapTarget(t *Tunnel) (Node, error) {
	cur := t.nodeOf()
	avoid := t.swapAvoid()
	picks, err := m.pickNodes(cur.CountryCode, 1, avoid)
	if err != nil && len(avoid) > 1 {
		// 这个地区的节点都换过一轮了。清掉历史重新开始，
		// 总比告诉用户"没得换了"好——转一圈之后原来那些节点未必还是当初的状态。
		t.forgetSwaps()
		picks, err = m.pickNodes(cur.CountryCode, 1, t.swapAvoid())
	}
	if err != nil {
		return Node{}, err
	}
	t.rememberSwap(cur.HostName)
	t.rememberSwap(picks[0].HostName)
	return picks[0], nil
}

// StopAll 停掉所有隧道并清空状态文件。
func (m *Manager) StopAll() {
	for _, t := range m.Tunnels() {
		_ = m.Stop(t.Slot)
	}
}

// SetCred 改一条出口的 SOCKS5 凭据。cred 两个字段都为空表示随机重置。
func (m *Manager) SetCred(slot int, cred SocksCred) (SocksCred, error) {
	m.mu.RLock()
	t, ok := m.tunnels[slot]
	m.mu.RUnlock()
	if !ok {
		return SocksCred{}, fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}

	if cred.User == "" && cred.Pass == "" {
		gen, err := newSocksCred()
		if err != nil {
			return SocksCred{}, err
		}
		cred = gen
	}
	if err := validateCred(cred); err != nil {
		return SocksCred{}, err
	}

	t.setCredential(cred)
	if err := m.saveState(); err != nil {
		log.Printf("保存状态失败: %v", err)
	}
	return cred, nil
}

// Shutdown 停掉运行态但保留状态文件，让下次启动能恢复同样的隧道。
func (m *Manager) Shutdown() {
	for _, t := range m.Tunnels() {
		t.stop()
	}
}

// prepareHost 打开转发开关。netns 出网依赖它。
func prepareHost() error {
	if err := cmdRun(exec.Command("sysctl", "-qw", "net.ipv4.ip_forward=1")); err != nil {
		return fmt.Errorf("开启 ip_forward 失败: %w", err)
	}
	return nil
}

// nodeInUse 判断某节点是否已被别的隧道占用。
func (m *Manager) nodeInUse(host string, exceptSlot int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for slot, t := range m.tunnels {
		if slot != exceptSlot && t.nodeOf().HostName == host {
			return true
		}
	}
	return false
}