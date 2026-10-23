package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cloudflare WARP 出口。
//
// 用官方的 warp-cli / warp-svc，跑在它的代理模式（mode proxy）下：只在 127.0.0.1 上开
// 一个本地 SOCKS5 端口，不建 tun、不改路由，所以不会动到母机的网络。这和 Psiphon 的
// 接法一样——母机上的混合端口收到连接后，转手经这个本地端口出去。
//
// 几点限制，界面和 README 都要讲清楚：
//   - warp-svc 整台机器只有一个，本地端口也固定，所以 WARP 出口最多一个。
//   - 只转 TCP，WARP 的代理模式没有 UDP。
//   - 出口 IP 是 Cloudflare 的，地区跟着母机走，不能选。
//   - 域名交给 WARP 解析，母机上不发 DNS 查询。

const providerWarp = "warp"

// warpProxyPort 是 warp-svc 代理模式监听的本地端口，官方默认值。
const warpProxyPort = 40000

// warpReadyTimeout 是等 WARP 连通的上限。第一次要先向 Cloudflare 注册设备。
const warpReadyTimeout = 60 * time.Second

// warpSwapTries 是换节点时最多重新注册几次。重新注册换一个设备身份，
// Cloudflare 可能给另一个出口 IP，也可能还是同一个，所以最多试几次就用着。
const warpSwapTries = 3

const warpLabel = "Cloudflare WARP"

// warpNode 把 WARP 出口包装成一个 Node，好和别的出口走同一套管理逻辑。
func warpNode() Node {
	return Node{Provider: providerWarp, HostName: "cloudflare-warp"}
}

var (
	// warpMu 把对 warp-cli 的一整套操作串起来：它背后是同一个守护进程，
	// 注册、改模式、连接穿插着跑会互相踩。
	warpMu sync.Mutex

	warpDaemonMu     sync.Mutex
	warpDaemon       *exec.Cmd     // 我们自己拉起的 warp-svc；系统服务管着时为 nil
	warpDaemonExited chan struct{} // warpDaemon 退出（已被 Wait 回收）时关闭
)

// ---- warp-cli ----

// warpCli 跑一次 warp-cli 并返回合并输出。--accept-tos 每次都带上，免得卡在条款确认上。
func warpCli(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "warp-cli", append([]string{"--accept-tos"}, args...)...)
	out, err := cmdCombined(cmd)
	text := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() != nil {
			return text, fmt.Errorf("warp-cli %s 超时", strings.Join(args, " "))
		}
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("warp-cli %s: %s", strings.Join(args, " "), shortErr(text))
	}
	return text, nil
}

// warpUsageError 判断 warp-cli 的报错是不是"这个子命令不存在"。
// 2024 年前后子命令改过名（register → registration new 之类），新旧版本都要认。
func warpUsageError(out string) bool {
	low := strings.ToLower(out)
	for _, s := range []string{"unrecognized subcommand", "unexpected argument", "invalid subcommand", "which wasn't expected"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// warpCliAny 依次试几种写法，直到有一种不是"子命令不存在"。
func warpCliAny(timeout time.Duration, variants ...[]string) (string, error) {
	var out string
	var err error
	for _, args := range variants {
		out, err = warpCli(timeout, args...)
		if err == nil || !warpUsageError(out) {
			return out, err
		}
	}
	return out, err
}

// parseWarpStatus 把 warp-cli status 的输出归成 connected / connecting / disconnected / unknown。
// 形如 "Status update: Connected"、"Status update: Disconnected. Reason: Manual Disconnection"。
func parseWarpStatus(out string) string {
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "disconnected"):
		return "disconnected"
	case strings.Contains(low, "connecting"):
		return "connecting"
	case strings.Contains(low, "connected"):
		return "connected"
	}
	return "unknown"
}

// ---- 安装与守护进程 ----

// warpSupported 判断这台机器能不能开 WARP 出口。
func warpSupported() error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return fmt.Errorf("Cloudflare WARP 官方客户端只有 Linux 的 amd64 / arm64 版本，这台是 %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return nil
}

func warpInstalled() bool {
	for _, b := range []string{"warp-cli", "warp-svc"} {
		if _, err := exec.LookPath(b); err != nil {
			return false
		}
	}
	return true
}

// osRelease 读 /etc/os-release 里的键值。
func osRelease() map[string]string {
	out := map[string]string{}
	blob, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(blob), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			out[k] = strings.Trim(v, `"'`)
		}
	}
	return out
}

// warpInstallScript 按发行版给出安装 cloudflare-warp 的脚本，不认识的发行版返回错误。
func warpInstallScript(rel map[string]string) (string, error) {
	family := strings.ToLower(rel["ID"] + " " + rel["ID_LIKE"])
	has := func(words ...string) bool {
		for _, w := range words {
			for _, f := range strings.Fields(family) {
				if f == w {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("debian", "ubuntu"):
		codename := rel["VERSION_CODENAME"]
		if codename == "" {
			return "", fmt.Errorf("读不到发行版代号（VERSION_CODENAME），请手动安装 cloudflare-warp")
		}
		return `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq curl ca-certificates
key=/usr/share/keyrings/cloudflare-warp.asc
curl -fsSL https://pkg.cloudflareclient.com/pubkey.gpg -o "$key"
if command -v gpg >/dev/null 2>&1; then
  gpg --yes --dearmor -o /usr/share/keyrings/cloudflare-warp.gpg "$key"
  key=/usr/share/keyrings/cloudflare-warp.gpg
fi
echo "deb [signed-by=$key] https://pkg.cloudflareclient.com/ ` + codename + ` main" > /etc/apt/sources.list.d/cloudflare-client.list
apt-get update -qq
apt-get install -y -qq cloudflare-warp
`, nil
	case has("rhel", "fedora", "centos", "rocky", "almalinux", "amzn"):
		return `set -e
mgr=dnf; command -v dnf >/dev/null 2>&1 || mgr=yum
curl -fsSL https://pkg.cloudflareclient.com/cloudflare-warp-ascii.repo -o /etc/yum.repos.d/cloudflare-warp.repo
$mgr install -y cloudflare-warp
`, nil
	}
	return "", fmt.Errorf("这个发行版（%s）没有 Cloudflare 官方包，请手动安装 cloudflare-warp", strings.TrimSpace(family))
}

// installWarp 按需装官方包。第一次开 WARP 出口时调用，装完之后不会再来。
func installWarp() error {
	script, err := warpInstallScript(osRelease())
	if err != nil {
		return err
	}
	log.Printf("正在安装 cloudflare-warp（Cloudflare 官方软件源）...")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	out, err := cmdCombined(exec.CommandContext(ctx, "bash", "-c", script))
	if err != nil {
		return fmt.Errorf("安装 cloudflare-warp 失败: %s", shortErr(lastNonEmptyLine(string(out), err.Error())))
	}
	if !warpInstalled() {
		return fmt.Errorf("装完了但找不到 warp-cli / warp-svc")
	}
	log.Printf("cloudflare-warp 已安装")
	return nil
}

func lastNonEmptyLine(s, fallback string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return fallback
}

// ensureWarpDaemon 确保 warp-svc 在跑：系统服务已经起了就直接用；
// 有 systemd 就让它起；都不行（容器、OpenRC）就自己拉一个。
func ensureWarpDaemon(workDir string) error {
	alive := func() bool {
		_, err := warpCli(10*time.Second, "status")
		return err == nil
	}
	if alive() {
		return nil
	}
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		_ = cmdRun(exec.Command("systemctl", "start", "warp-svc"))
		if waitFor(15*time.Second, alive) {
			return nil
		}
	}

	dir := filepath.Join(workDir, "warp")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(dir, "warp-svc.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command("warp-svc")
	cmd.Stdout, cmd.Stderr = logf, logf
	// 它开的本地端口要落在母机的 loopback 上，所以和 psiphon 一样从母机命名空间里起
	if err := cmdStart(cmd); err != nil {
		return fmt.Errorf("启动 warp-svc 失败: %w", err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	warpDaemonMu.Lock()
	warpDaemon, warpDaemonExited = cmd, exited
	warpDaemonMu.Unlock()

	if !waitFor(20*time.Second, alive) {
		why := lastLogLine(filepath.Join(dir, "warp-svc.log"))
		stopWarpDaemon()
		if why != "" {
			return fmt.Errorf("warp-svc 没起来: %s", why)
		}
		return fmt.Errorf("warp-svc 没起来，详见 %s", filepath.Join(dir, "warp-svc.log"))
	}
	return nil
}

// waitFor 每秒试一次，等到 ok 或超时。
func waitFor(total time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(total)
	for {
		if ok() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Second)
	}
}

// stopWarpDaemon 结束我们自己拉起的 warp-svc。系统服务管着的那个不动。
func stopWarpDaemon() {
	warpDaemonMu.Lock()
	cmd, exited := warpDaemon, warpDaemonExited
	warpDaemon, warpDaemonExited = nil, nil
	warpDaemonMu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
	}
}

// ---- 隧道 ----

// startWarp 让 WARP 连上并进入代理模式。
//
// 顺序有讲究：先切到代理模式再连接。默认的 warp 模式会接管整机路由，
// 一旦在远程机器上先连了，SSH 就断了。
func (t *Tunnel) startWarp(workDir string) error {
	warpMu.Lock()
	defer warpMu.Unlock()

	if err := warpSupported(); err != nil {
		return err
	}
	if !warpInstalled() {
		if err := installWarp(); err != nil {
			return err
		}
	}
	if err := ensureWarpDaemon(workDir); err != nil {
		return err
	}
	if _, err := warpCliAny(20*time.Second, []string{"mode", "proxy"}, []string{"set-mode", "proxy"}); err != nil {
		return err
	}

	if t.takeFresh() {
		// 换节点：丢掉当前的设备身份重新注册，Cloudflare 可能给另一个出口 IP
		_, _ = warpCli(20*time.Second, "disconnect")
		_, _ = warpCliAny(20*time.Second, []string{"registration", "delete"}, []string{"delete"})
	}
	if _, err := warpCliAny(20*time.Second, []string{"registration", "show"}, []string{"account"}); err != nil {
		if _, err := warpCliAny(60*time.Second, []string{"registration", "new"}, []string{"register"}); err != nil {
			return err
		}
	}
	port := strconv.Itoa(warpProxyPort)
	if _, err := warpCliAny(20*time.Second, []string{"proxy", "port", port}, []string{"set-proxy-port", port}); err != nil {
		return err
	}
	if _, err := warpCli(20*time.Second, "connect"); err != nil {
		return err
	}

	deadline := time.Now().Add(warpReadyTimeout)
	last := ""
	for time.Now().Before(deadline) {
		if t.statusOf() == "stopped" {
			_, _ = warpCli(20*time.Second, "disconnect")
			return fmt.Errorf("出口已停止")
		}
		out, err := warpCli(10*time.Second, "status")
		if err == nil {
			last = out
			if parseWarpStatus(out) == "connected" {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("%.0f 秒内没连上 WARP（%s）", warpReadyTimeout.Seconds(), shortErr(lastNonEmptyLine(last, "无状态输出")))
}

// killWarp 断开 WARP 连接。守护进程留着：下次连起来快，系统服务管着的也不该被我们停掉。
func (t *Tunnel) killWarp() {
	if !t.isWarp() {
		return
	}
	warpMu.Lock()
	defer warpMu.Unlock()
	if !warpInstalled() {
		return
	}
	_, _ = warpCli(20*time.Second, "disconnect")
}

// warpDial 经 warp-svc 的本地 SOCKS5 端口连到 addr。只转 TCP。
func (t *Tunnel) warpDial(network, addr string) (net.Conn, error) {
	if strings.HasPrefix(network, "udp") {
		return nil, errWarpUDP
	}
	return dialLocalSocks(warpProxyPort, addr)
}

// errWarpUDP 是 WARP 出口被要求走 UDP 时的错误：代理模式只有 TCP。
var errWarpUDP = fmt.Errorf("WARP 出口不支持 UDP")
