package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Psiphon 出口。
//
// 和 VPN Gate 那条路完全不同：psiphon-tunnel-core 是个用户态客户端，不建 tun、
// 不改路由，连上之后只在 127.0.0.1 上开一个本地 SOCKS 端口。所以这里不用 netns，
// 母机上的混合端口收到连接后，转手经这个本地 SOCKS 端口出去就行。
//
// 带来的几个差别，界面和 README 都要讲清楚：
//   - 服务器由 psiphon 在地区内自己挑，出口 IP 会随它重连而变化，不像 VPN Gate 那样钉死一个节点。
//   - 只转 TCP。psiphon 的本地 SOCKS 不支持 UDP ASSOCIATE。
//   - 出口是 Psiphon 的机房 IP，不是家宽。
//   - 域名交给 psiphon 服务器去解析，母机上不做 DNS 查询。

const providerPsiphon = "psiphon"

// 官方只发布 linux x86_64 的预编译客户端，而且主分支一两周就更新一次。
// 跟着主分支走的话，校验和一变就全部下载失败，所以钉死在某个提交上。
// 升级时两个值一起换：先下载新提交的文件，sha256sum 核对后再改这里。
const (
	psiphonBinCommit = "33ff79e89260f3adf92e846b96b3369b86a819d8"
	psiphonBinSHA256 = "046ca168da25d2c0e61c1c15e2b73bfbb67f5208112a4c94fe58fc5aab784f19"
)

var psiphonBinURL = "https://raw.githubusercontent.com/Psiphon-Labs/psiphon-tunnel-core-binaries/" +
	psiphonBinCommit + "/linux/psiphon-tunnel-core-x86_64"

// psiphonBinSum 单独成变量，测试里要能换成假文件的校验和。
var psiphonBinSum = psiphonBinSHA256

// psiphonReadyTimeout 是等 psiphon 连通的上限。全新的 datastore 要先去 S3
// 拉远程服务器清单再握手，慢的线路要十几二十秒，留足余量。
const psiphonReadyTimeout = 60 * time.Second

// psiphonSwapTries 是换节点时最多重挑几次服务器。psiphon 没有"排除某台服务器"
// 的参数，只能清掉 datastore 让它重新挑，碰巧又分到旧 IP 就再来。
const psiphonSwapTries = 3

// psiphonNoticesMax 是通知文件的上限。开着诊断通知，长时间跑下来会一直变大，
// psiphon 用 O_APPEND 写，从外面截断是安全的。
const psiphonNoticesMax = 2 << 20

// psiphonDefaultRegions 是还没从 psiphon 那里拿到实时清单时用的地区列表，
// 与 Linphon 的 regions.txt 一致。任意一条 psiphon 连上之后会报
// AvailableEgressRegions，那时换成实时清单（见 setPsiphonRegions）。
var psiphonDefaultRegions = []string{
	"AT", "BE", "BG", "CA", "CH", "CZ", "DE", "DK", "EE", "ES", "FI", "FR", "GB", "HU",
	"IE", "IN", "IT", "JP", "LV", "NL", "NO", "PL", "RO", "RS", "SE", "SG", "SK", "US",
}

var (
	psiphonRegionsMu  sync.RWMutex
	psiphonRegionsCur = append([]string(nil), psiphonDefaultRegions...)
)

// psiphonRegions 返回当前可选的出口地区，已排序。
func psiphonRegions() []string {
	psiphonRegionsMu.RLock()
	defer psiphonRegionsMu.RUnlock()
	return append([]string(nil), psiphonRegionsCur...)
}

// setPsiphonRegions 用 psiphon 报上来的实时清单替换地区列表。
// 空清单不替换：那多半是还没拉到服务器列表，不代表真的一个地区都没有。
func setPsiphonRegions(regions []string) {
	var out []string
	seen := map[string]bool{}
	for _, r := range regions {
		r = strings.ToUpper(strings.TrimSpace(r))
		if validRegionCode(r) && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return
	}
	sort.Strings(out)
	psiphonRegionsMu.Lock()
	psiphonRegionsCur = out
	psiphonRegionsMu.Unlock()
}

// validRegionCode 判断是不是两位大写字母的国家码。
func validRegionCode(code string) bool {
	if len(code) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return false
		}
	}
	return true
}

// psiphonNode 把"某地区的 Psiphon 出口"包装成一个 Node，好和 VPN Gate 节点走同一套管理逻辑。
// region 为空表示不限地区，由 psiphon 挑最快的。
//
// HostName 只是个标签：psiphon 的服务器不归我们挑，同地区开几条都行，
// 不会和 VPN Gate 的主机名撞在一起，也不参与"节点被占用"的判断。
func psiphonNode(region string) Node {
	region = strings.ToUpper(strings.TrimSpace(region))
	host := "psiphon"
	if region != "" {
		host += "-" + strings.ToLower(region)
	}
	return Node{Provider: providerPsiphon, HostName: host, CountryCode: region}
}

// psiphonRegionText 给 Psiphon 出口的地区起个人能读的名字，不限地区时说清楚。
func psiphonRegionText(n Node) string {
	if place := nodeLabel(n); place != "" {
		return place
	}
	return "不限地区"
}

// psiphonConfig 是喂给 psiphon-tunnel-core 的配置。字段名就是 psiphon 认的 JSON 键。
//
// 前四项是接入 Psiphon 网络的公共参数，与 Linphon 的 psiphon.config 一致。
type psiphonConfig struct {
	PropagationChannelId               string
	SponsorId                          string
	RemoteServerListUrl                string
	RemoteServerListSignaturePublicKey string
	EgressRegion                       string `json:",omitempty"`
	// 0 表示让系统挑空闲端口，选中的端口会以 ListeningSocksProxyPort 通知报出来。
	// 自己挑再传进去会有"挑完到绑上"之间被别人占走的窗口。
	LocalSocksProxyPort int
	// 只用 SOCKS，HTTP 代理端口开着也是白占一个口子
	DisableLocalHTTPProxy bool
	// 打开才能看到 Error/Warning 一类通知，连不上时要靠它们说清原因
	EmitDiagnosticNotices bool
}

func renderPsiphonConfig(region string) ([]byte, error) {
	cfg := psiphonConfig{
		PropagationChannelId:               "FFFFFFFFFFFFFFFF",
		SponsorId:                          "FFFFFFFFFFFFFFFF",
		RemoteServerListUrl:                "https://s3.amazonaws.com//psiphon/web/mjr4-p23r-puwl/server_list_compressed",
		RemoteServerListSignaturePublicKey: "MIICIDANBgkqhkiG9w0BAQEFAAOCAg0AMIICCAKCAgEAt7Ls+/39r+T6zNW7GiVpJfzq/xvL9SBH5rIFnk0RXYEYavax3WS6HOD35eTAqn8AniOwiH+DOkvgSKF2caqk/y1dfq47Pdymtwzp9ikpB1C5OfAysXzBiwVJlCdajBKvBZDerV1cMvRzCKvKwRmvDmHgphQQ7WfXIGbRbmmk6opMBh3roE42KcotLFtqp0RRwLtcBRNtCdsrVsjiI1Lqz/lH+T61sGjSjQ3CHMuZYSQJZo/KrvzgQXpkaCTdbObxHqb6/+i1qaVOfEsvjoiyzTxJADvSytVtcTjijhPEV6XskJVHE1Zgl+7rATr/pDQkw6DPCNBS1+Y6fy7GstZALQXwEDN/qhQI9kWkHijT8ns+i1vGg00Mk/6J75arLhqcodWsdeG/M/moWgqQAnlZAGVtJI1OgeF5fsPpXu4kctOfuZlGjVZXQNW34aOzm8r8S0eVZitPlbhcPiR4gT/aSMz/wd8lZlzZYsje/Jr8u/YtlwjjreZrGRmG8KMOzukV3lLmMppXFMvl4bxv6YFEmIuTsOhbLTwFgh7KYNjodLj/LsqRVfwz31PgWQFTEPICV7GCvgVlPRxnofqKSjgTWI4mxDhBpVcATvaoBl1L/6WLbFvBsoAUBItWwctO2xalKxF5szhGm8lccoc5MZr8kfE0uxMgsxz4er68iCID+rsCAQM=",
		EgressRegion:                       region,
		LocalSocksProxyPort:                0,
		DisableLocalHTTPProxy:              true,
		EmitDiagnosticNotices:              true,
	}
	blob, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(blob, '\n'), nil
}

// ---- 二进制 ----

var (
	psiphonBinMu   sync.Mutex
	psiphonBinPath string // 校验通过后缓存下来，别每开一条出口都把 10MB 哈希一遍
)

// psiphonBinary 返回可用的 psiphon-tunnel-core 路径，本地没有就下载。
//
// 优先用环境变量 FANOUT_PSIPHON_BIN 指定的文件（不校验：那是用户自己负责的，
// 比如 arm64 机器上自己编译的客户端）。否则放在 <workDir>/bin 下，按钉死的
// sha256 校验；校验不过（文件损坏、或者 fanout 升级换了版本）就重新下载。
//
// 并发开多条出口时只下载一次：后来的等锁，拿到的就是前面下好的。
func psiphonBinary(workDir string) (string, error) {
	if p := strings.TrimSpace(os.Getenv("FANOUT_PSIPHON_BIN")); p != "" {
		if info, err := os.Stat(p); err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("FANOUT_PSIPHON_BIN 指向的 %s 不是可用的文件", p)
		}
		return p, nil
	}

	psiphonBinMu.Lock()
	defer psiphonBinMu.Unlock()
	if psiphonBinPath != "" {
		return psiphonBinPath, nil
	}

	dest := filepath.Join(workDir, "bin", "psiphon-tunnel-core")
	if fileSHA256(dest) == psiphonBinSum {
		psiphonBinPath = dest
		return dest, nil
	}
	if err := psiphonSupported(); err != nil {
		return "", err
	}

	log.Printf("正在下载 psiphon-tunnel-core ...")
	if err := downloadPsiphon(dest); err != nil {
		return "", err
	}
	log.Printf("psiphon-tunnel-core 已就绪: %s", dest)
	psiphonBinPath = dest
	return dest, nil
}

// psiphonSupported 判断这台机器能不能开 Psiphon 出口。新建时先问一句，
// 免得开了一排出口才一条条报同一个错。
func psiphonSupported() error {
	if strings.TrimSpace(os.Getenv("FANOUT_PSIPHON_BIN")) != "" || runtime.GOARCH == "amd64" {
		return nil
	}
	return fmt.Errorf("Psiphon 官方只发布 x86_64 的 Linux 客户端，这台是 %s；"+
		"可以自己编译 psiphon-tunnel-core 的 ConsoleClient，再用环境变量 FANOUT_PSIPHON_BIN 指过去", runtime.GOARCH)
}

// downloadPsiphon 下载、校验并原子替换到 dest。校验不过不落盘。
func downloadPsiphon(dest string) error {
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(psiphonBinURL)
	if err != nil {
		return fmt.Errorf("下载 psiphon-tunnel-core 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 psiphon-tunnel-core 失败: HTTP %d", resp.StatusCode)
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("下载 psiphon-tunnel-core 失败: %w", err)
	}
	sum := sha256.Sum256(blob)
	if got := hex.EncodeToString(sum[:]); got != psiphonBinSum {
		return fmt.Errorf("psiphon-tunnel-core 校验和不符（拿到 %s），拒绝使用", got)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, blob, 0755); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// fileSHA256 算文件的 sha256，读不了返回空串。
func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ---- 进程 ----

// psiphonDir 是这条出口的工作目录：配置、通知、日志和 datastore 都在这里。
//
// 按槽位命名，重启后同一条出口还用同一个 datastore：psiphon 会优先重连上次
// 连上的服务器，出口 IP 大概率不变，已分发的客户端感觉不到。
func (t *Tunnel) psiphonDir(workDir string) string {
	return filepath.Join(workDir, "psiphon", strconv.Itoa(t.Slot))
}

// startPsiphon 拉起 psiphon-tunnel-core，等它连通并报出本地 SOCKS 端口。
func (t *Tunnel) startPsiphon(workDir string) error {
	bin, err := psiphonBinary(workDir)
	if err != nil {
		return err
	}

	dir := t.psiphonDir(workDir)
	data := filepath.Join(dir, "data")
	// 上一次的 fanout 要是没收拾干净就退了，它拉起的 psiphon 还占着 datastore 的文件锁，
	// 新进程会卡在打开 datastore 上，先把它清掉
	killStalePsiphon(data)
	if t.takeFresh() {
		// 换节点：忘掉上次连的服务器，让 psiphon 重新挑
		_ = os.RemoveAll(data)
	}
	if err := os.MkdirAll(data, 0700); err != nil {
		return fmt.Errorf("创建 %s 失败: %w", data, err)
	}

	cfg, err := renderPsiphonConfig(t.nodeOf().CountryCode)
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, cfg, 0600); err != nil {
		return fmt.Errorf("写配置失败: %w", err)
	}
	notices := filepath.Join(dir, "notices.jsonl")
	if err := os.WriteFile(notices, nil, 0600); err != nil {
		return fmt.Errorf("写通知文件失败: %w", err)
	}
	logPath := filepath.Join(dir, "psiphon.log")
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("打开日志失败: %w", err)
	}
	defer logf.Close()

	cmd := exec.Command(bin,
		"-config", cfgPath,
		"-dataRootDirectory", data,
		"-notices", notices,
	)
	cmd.Dir = dir
	cmd.Stdout = logf
	cmd.Stderr = logf
	// 和 openvpn 一样从母机命名空间里起：它开的本地 SOCKS 端口必须落在母机的
	// loopback 上，掉进某条隧道的 netns 里就谁也连不上（见 netnsguard.go）
	if err := cmdStart(cmd); err != nil {
		return fmt.Errorf("启动 psiphon 失败: %w", err)
	}
	// 进程退出通过 channel 通知，不去读 cmd.ProcessState（那是 Wait 在别的 goroutine 里写的）。
	// exited 给 killPsiphon 用：杀完要等它真退出、放掉 datastore 的锁
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		done <- cmd.Wait()
		close(exited)
	}()

	t.mu.Lock()
	stopped := t.status == "stopped"
	t.psi = cmd
	t.psiExited = exited
	t.psiPort = 0
	t.mu.Unlock()
	if stopped {
		// 起进程这一会儿用户把出口停了：stop() 那时还看不到这个进程，这里自己收掉
		t.killPsiphon()
		return fmt.Errorf("出口已停止")
	}

	deadline := time.Now().Add(psiphonReadyTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			st := readPsiphonNotices(notices)
			if why := st.lastErr; why != "" {
				return fmt.Errorf("psiphon 提前退出: %s", why)
			}
			if why := lastLogLine(logPath); why != "" {
				return fmt.Errorf("psiphon 提前退出: %s", why)
			}
			return fmt.Errorf("psiphon 提前退出，详见 %s", logPath)
		default:
		}
		if t.statusOf() == "stopped" {
			t.killPsiphon()
			return fmt.Errorf("出口已停止")
		}
		st := readPsiphonNotices(notices)
		if st.portInUse {
			return fmt.Errorf("psiphon 的本地 SOCKS 端口被占用")
		}
		if st.socksPort > 0 && st.tunnels > 0 {
			setPsiphonRegions(st.regions)
			t.mu.Lock()
			t.psiPort = st.socksPort
			t.mu.Unlock()
			return nil
		}
		time.Sleep(time.Second)
	}

	st := readPsiphonNotices(notices)
	if why := st.lastErr; why != "" {
		return fmt.Errorf("%.0f 秒内没连上 Psiphon（最后提示：%s）", psiphonReadyTimeout.Seconds(), why)
	}
	return fmt.Errorf("%.0f 秒内没连上 Psiphon，详见 %s", psiphonReadyTimeout.Seconds(), notices)
}

// killPsiphon 结束这条出口的 psiphon 进程，并等它真的退出。
//
// 不等的话，换节点、重连紧接着在同一个目录上起新进程时，旧进程可能还没走完、
// 还锁着 datastore——真机上就撞到过新进程起来时旧的那个还在。
// 等到 Wait 返回也顺带把它回收了，fanout 自己退出时不会留下僵尸。
func (t *Tunnel) killPsiphon() {
	t.mu.Lock()
	cmd, exited := t.psi, t.psiExited
	t.psi, t.psiExited, t.psiPort = nil, nil, 0
	t.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	if exited != nil {
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			log.Printf("隧道 %d 的 psiphon 进程 %d 5 秒内没退出", t.Slot, cmd.Process.Pid)
		}
	}
}

// markFresh 让下次启动前清空 datastore，psiphon 就会重新挑服务器。
func (t *Tunnel) markFresh() {
	t.mu.Lock()
	t.psiFresh = true
	t.mu.Unlock()
}

// takeFresh 读出并清掉 markFresh 打的标记。
func (t *Tunnel) takeFresh() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	fresh := t.psiFresh
	t.psiFresh = false
	return fresh
}

// trimPsiphonNotices 通知文件超过上限就清空，健康检查顺手调用。
func (t *Tunnel) trimPsiphonNotices(workDir string) {
	path := filepath.Join(t.psiphonDir(workDir), "notices.jsonl")
	if info, err := os.Stat(path); err == nil && info.Size() > psiphonNoticesMax {
		_ = os.Truncate(path, 0)
	}
}

// killStalePsiphon 结束所有用着 dataDir 的 psiphon 进程。
//
// 按命令行里的 -dataRootDirectory 认人，只动用着这个目录的，
// 不会误伤同机上别的程序（包括 Linphon 起的 psiphon）。
func killStalePsiphon(dataDir string) {
	victims := psiphonPidsUsing(dataDir)
	for _, pid := range victims {
		log.Printf("结束残留的 psiphon 进程 %d（%s）", pid, dataDir)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	// 等它们真的退出、放掉文件锁。按命令行重新找一遍而不是 kill(pid, 0)：
	// 没人回收的僵尸对 kill(pid, 0) 也算活着，但它的命令行已经读不出来了
	for i := 0; i < 30 && len(victims) > 0; i++ {
		time.Sleep(100 * time.Millisecond)
		victims = psiphonPidsUsing(dataDir)
	}
}

// psiphonPidsUsing 找出命令行里 -dataRootDirectory 是 dataDir 的进程。
func psiphonPidsUsing(dataDir string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		if argvHasFlagValue(strings.Split(string(bytes.TrimRight(raw, "\x00")), "\x00"), "-dataRootDirectory", dataDir) {
			out = append(out, pid)
		}
	}
	return out
}

func argvHasFlagValue(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

// ---- 通知 ----

// psiphonStatus 是从通知文件里读出来的状态。
type psiphonStatus struct {
	socksPort int      // ListeningSocksProxyPort
	tunnels   int      // 最近一条 Tunnels 通知里的隧道数
	regions   []string // AvailableEgressRegions
	portInUse bool     // SocksProxyPortInUse
	lastErr   string   // 最近一条 Error / Alert / Warning
}

// readPsiphonNotices 读通知文件。读不到就当什么都还没发生。
func readPsiphonNotices(path string) psiphonStatus {
	f, err := os.Open(path)
	if err != nil {
		return psiphonStatus{}
	}
	defer f.Close()
	return parsePsiphonNotices(f)
}

// parsePsiphonNotices 解析 psiphon 的通知流：一行一个 JSON，
// 形如 {"noticeType":"Tunnels","data":{"count":1},"timestamp":"..."}。
func parsePsiphonNotices(r io.Reader) psiphonStatus {
	var st psiphonStatus
	sc := bufio.NewScanner(r)
	// 有些通知（服务器清单、诊断信息）一行很长，默认 64KB 的缓冲装不下会直接停止扫描
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var n struct {
			NoticeType string `json:"noticeType"`
			Data       struct {
				Port    int      `json:"port"`
				Count   int      `json:"count"`
				Regions []string `json:"regions"`
				Message string   `json:"message"`
			} `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &n) != nil {
			continue
		}
		switch n.NoticeType {
		case "ListeningSocksProxyPort":
			st.socksPort = n.Data.Port
		case "SocksProxyPortInUse":
			st.portInUse = true
		case "Tunnels":
			st.tunnels = n.Data.Count
		case "AvailableEgressRegions":
			st.regions = n.Data.Regions
		case "Error", "Alert", "Warning":
			msg := strings.TrimSpace(n.Data.Message)
			// 连上之后 psiphon 会经隧道拉一次 DSL 服务器清单，公开配置里没有它要的签名公钥，
			// 必报一条 "tunneled DSL fetch failed: ... missing public key"。不影响使用，
			// 记下来的话以后真连不上时会拿它当原因，把人带偏
			if msg != "" && !strings.Contains(msg, "DSL fetch failed") {
				st.lastErr = msg
			}
		}
	}
	return st
}

// ---- 出站 ----

// errUDPUnsupported 是 Psiphon 出口被要求走 UDP 时的错误：它的本地 SOCKS 只转 TCP。
var errUDPUnsupported = errors.New("Psiphon 出口不支持 UDP")

// psiphonPortOf 返回 psiphon 当前的本地 SOCKS 端口，没连上时为 0。
func (t *Tunnel) psiphonPortOf() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.psiPort
}

// psiphonDial 经 psiphon 的本地 SOCKS 端口连到 addr。
//
// 端口每次现取：psiphon 重启后端口会变，监听却一直是同一个，不能把旧端口捏在闭包里。
// 目标是域名就原样交给 psiphon，由它的服务器解析，母机上不发 DNS 查询。
func (t *Tunnel) psiphonDial(network, addr string) (net.Conn, error) {
	if strings.HasPrefix(network, "udp") {
		return nil, errUDPUnsupported
	}
	port := t.psiphonPortOf()
	if port == 0 {
		return nil, fmt.Errorf("Psiphon 还没连上")
	}
	return dialLocalSocks(port, addr)
}

// dialLocalSocks 经母机 127.0.0.1:port 上的本地 SOCKS5 代理（psiphon、warp-svc）连到 addr。
func dialLocalSocks(port int, addr string) (net.Conn, error) {
	var conn net.Conn
	// 只有建 socket 这一步要在母机命名空间里做：线程要是被某条隧道带进了它的 netns，
	// 连 127.0.0.1 连到的就是那个 netns 的 loopback，psiphon 不在那里（见 netnsguard.go）
	err := inMainNetns(func() error {
		c, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
		conn = c
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("连不上本地代理端口 %d: %w", port, err)
	}
	if err := socks5ClientConnect(conn, addr, 30*time.Second); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// socks5ClientConnect 在已建立的连接上做一次无认证的 SOCKS5 CONNECT。
func socks5ClientConnect(conn net.Conn, addr string, timeout time.Duration) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("目标地址不合法: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("目标端口不合法: %s", portStr)
	}

	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})

	if _, err := conn.Write([]byte{socksVer5, 1, authNone}); err != nil {
		return err
	}
	var hello [2]byte
	if _, err := io.ReadFull(conn, hello[:]); err != nil {
		return fmt.Errorf("SOCKS5 握手失败: %w", err)
	}
	if hello[0] != socksVer5 || hello[1] != authNone {
		return fmt.Errorf("SOCKS5 握手被拒（%#x %#x）", hello[0], hello[1])
	}

	req := []byte{socksVer5, cmdConnect, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, atypIPv4)
			req = append(req, v4...)
		} else {
			req = append(req, atypIPv6)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return fmt.Errorf("域名长度不合法: %q", host)
		}
		req = append(req, atypDomain, byte(len(host)))
		req = append(req, host...)
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := conn.Write(req); err != nil {
		return err
	}

	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return fmt.Errorf("读 SOCKS5 应答失败: %w", err)
	}
	if head[1] != repSuccess {
		return fmt.Errorf("上游代理连接 %s 失败（SOCKS5 应答 %d）", addr, head[1])
	}
	// 跳过应答里的绑定地址，后面就是目标的数据流了
	var skip int
	switch head[3] {
	case atypIPv4:
		skip = 4 + 2
	case atypIPv6:
		skip = 16 + 2
	case atypDomain:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return err
		}
		skip = int(l[0]) + 2
	default:
		return fmt.Errorf("SOCKS5 应答地址类型未知: %d", head[3])
	}
	if _, err := io.CopyN(io.Discard, conn, int64(skip)); err != nil {
		return err
	}
	return nil
}

// probeExitIPVia 经给定的出站方式查询出口 IP。
func probeExitIPVia(dial dialFunc, timeout time.Duration) (string, error) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
				return dial(network, addr)
			},
			DisableKeepAlives: true,
		},
	}
	resp, err := client.Get("http://api.ipify.org")
	if err != nil {
		return "", fmt.Errorf("查询出口 IP 失败: %w", err)
	}
	defer resp.Body.Close()
	blob, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", fmt.Errorf("查询出口 IP 失败: %w", err)
	}
	ip := strings.TrimSpace(string(blob))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("出口 IP 返回异常: %q", ip)
	}
	return ip, nil
}
