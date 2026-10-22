# fanout

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

把 VPN Gate 的公共节点变成本地代理端口：一个端口一个出口 IP。
每个端口都是**混合端口**，SOCKS5 和 HTTP 代理两种协议都收，客户端连哪个端口，
就从哪个国家出去。

除了 VPN Gate，也可以开 [Psiphon](https://psiphon.ca/) 出口，见下面的
[Psiphon 出口](#psiphon-出口)一节。

![主界面](https://images.joeyblog.net/2026/7/27/fanout-dashboard.png)

四条隧道跑在一台机器上，四个端口对应四个国家的出口，母机自己的 IP 不受影响：

![出口验证](https://images.joeyblog.net/2026/7/26/fanout-6-exit-ip.png)

## 原理

每个节点跑在独立的 network namespace 里，netns 内启动官方 openvpn 客户端。
SOCKS5 监听在母机，出站连接用 `setns` 切进对应 netns 建立。

这样做的好处：VPN 的路由劫持只影响自己的 netns，不会切断母机的网络；
多个节点互不干扰，各自一个出口 IP。

```
客户端 ──> 母机 SOCKS5 :随机端口 ──> netns foN ──> openvpn ──> VPN Gate 节点
```

端口上两种协议同时可用：

- **SOCKS5**：CONNECT 与 UDP ASSOCIATE（RFC1928），DNS/QUIC 这类 UDP 也走隧道
- **HTTP 代理**：CONNECT 与普通请求（绝对 URL 形式）

两种协议共用同一套用户名/口令——SOCKS5 走 RFC1929，HTTP 走 `Proxy-Authorization: Basic`。
端口对公网敞开，没有口令等于谁扫到谁能用。

## 安装

需要 root，Linux（依赖 netns）。宿主必须放开 `/dev/net/tun`——不少 LXC 小鸡没给这个
权限，`ls /dev/net/tun` 不存在且 `mknod` 报 Operation not permitted 的话，这台机器用不了。

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/byJoey/fanout/main/install.sh)
```

会自动下载对应架构的预编译二进制。也可以 clone 仓库后在源码目录运行同一个脚本，
那样会从源码编译（需要 Go 1.21+，脚本会检查版本）。

依赖（openvpn / curl / openssl / iproute / iptables）会按发行版自动装，
apt、dnf、yum、pacman、apk、zypper 都认。服务用 systemd 或 OpenRC 都能装，
装完自动开机自启。不想动宿主环境也可以跑容器，见下面的 Docker 一节。

**Alpine** 默认不带 bash，先装一下：

```bash
apk add bash curl
bash <(curl -fsSL https://raw.githubusercontent.com/byJoey/fanout/main/install.sh)
```

装完会打印一条登录链接，之后敲 `f info`（或 `f link`）随时再看：

```
登录链接  http://<你的IP>:8899/?k=Zx9QpL2mVb7RkT4wYc8HnJ3sFd6GaE1u
```

在浏览器里打开一次就记住登录：服务端发一个 30 天的 cookie，常用的设备会自动续期，
重启、升级都不掉线。换设备就在那台设备上再打开一次链接。

链接里的密钥是 192 位随机数，存在 `/var/lib/fanout/key`。没有它的请求一律返回 404，
没有登录页、也没有表单可以爆破，扫端口的看不到这里跑着什么。
链接就是钥匙，别发给别人；泄露了就在设置页或 `f` 菜单里「重置登录链接」，
旧链接和所有设备的登录一起作废。

从老版本升上来时会自动生成密钥，原来的访问路径和口令不再使用（文件会被删掉），
用 `f info` 看新的登录链接。

### Docker

镜像在 `ghcr.io/fmys2010/howisp`，amd64 与 arm64 都有：

```bash
docker run -d --name fanout \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN --cap-add SYS_ADMIN \
  --security-opt apparmor=unconfined \
  --device /dev/net/tun \
  -v /var/lib/fanout:/var/lib/fanout \
  ghcr.io/fmys2010/howisp:latest
```

四点必须注意：

- 用 **host 网络**。fanout 建的 netns、iptables 规则和 SOCKS5 端口都落在宿主的网络命名空间里，
  桥接网络下随机分配的端口没法映射出去。
- 需要 **NET_ADMIN + SYS_ADMIN** 和 `/dev/net/tun`（图省事可以整体换成 `--privileged`）。
- 需要 **`--security-opt apparmor=unconfined`**。`ip netns add` 会调 `mount --make-shared`，
  Docker 默认的 AppArmor 策略会把它拦成 EPERM，隧道会以
  `mount --make-shared /run/netns failed: Permission denied` 失败。
- `/var/lib/fanout` 挂出来，重建容器后登录链接和隧道状态都还在。

容器里没有 `f` 菜单，登录链接这样看：

```bash
docker exec fanout fanout -link
```

宿主上要开着 `net.ipv4.ip_forward`（跑 Docker 的机器默认就是开的）。容器里 `/proc/sys` 是只读的，
fanout 会读一下当前值，已经是 1 就直接用；如果宿主上确实是 0，需要在宿主执行
`sysctl -w net.ipv4.ip_forward=1`。

仓库里带了开箱即用的 `docker-compose.yml`，直接：

```bash
docker compose up -d
```

想改成自己的配置（换端口、换卷路径、改成从源码构建），复制一份模板再改，
这样不会和仓库版本打架：

```bash
cp docker-compose.yml.example docker-compose.yml
```

停容器请用 `docker stop`——它会发 SIGTERM，fanout 会把 netns、veth 和 iptables 清干净。
`docker kill` 会留下宿主侧的 veth 和规则，不过下次启动恢复隧道时会自动收尾。

## 使用

界面以**出口**为单位：一行就是一条隧道加一个 SOCKS5 端口。

点「新建出口」，选地区和数量，提交后 fanout 会并行拉起隧道，进度按目标逐条回报。
地区里还有个「每个国家」：每个有空闲节点的国家各开几条，一次把所有地区铺开。
槽位不够时先开节点多的地区，不会中途报错。

![新建出口](https://images.joeyblog.net/2026/7/27/fanout-wizard.png)

每行右侧的按钮可以换一个节点（出口 IP 变、端口不变，已分发的客户端配置不用改），
或者停掉这个出口。换节点会避开这条出口之前用过的节点，连点几次每次都是新 IP。

点端口按钮能看到完整地址，`socks5://` 和 `http://` 两种形式都给出来，直接复制给客户端。
用哪个看客户端支持什么：只认 HTTP 代理的程序（比如 Python 的 urllib）就用 `http://` 那条。
口令可以单独重置，重置后两种地址一起失效。

### 地图与 IP 质量

界面顶部是一张世界地图：蓝点是母机，每个出口一个点（绿=已连通、黄=连接中、红=失败），
从母机连线到各个出口，有出口的国家会点亮。悬停看详情，点一下跳到对应那一行。
地图是嵌在程序里的 Natural Earth 国界，不加载任何在线瓦片或 CDN，离线也能看。

每个出口连通后，fanout 会**经这条出口**问一次 [IPPure](https://ippure.com/MyIP-Info-API.html)
的公开接口，拿到出口 IP 的城市、经纬度、ASN，以及：

- **风险系数**：0–100，越高越可疑。界面按 ≤30 / ≤60 / >60 标成低、中、高风险（分档是 fanout 自己定的）。
- **家宽 / 机房**：是不是运营商家庭宽带。
- **原生 / 广播**：IP 的注册地和实际所在地是否一致。

母机自己的 IP 也会查一次，显示在地图上方。结果按 IP 缓存 6 小时，出口 IP 不变就不重复查；
查失败 2 分钟后重试，同时最多查 3 个。行尾的仪表按钮可以立刻重查。
概况栏里的平均风险、家宽数、原生 IP 数只统计已经查到的出口。

这意味着母机 IP 和所有出口 IP 都会发给 IPPure（接口本来就是看来访 IP 的）。
IPPure 的接口还在测试阶段，查不到时界面显示「查询中」，不影响出口本身。

### 只用家宽

VPN Gate 的清单里混着一批它自己的机房服务器（`public-vpn-*` 和
`219.100.37.0/24`），出口一眼看得出是数据中心，还更容易满员。
设置里「只用家宽节点」默认开着，挑节点、地区可用数、自动重连的备选都只看
志愿者家宽。想连机房的一起用就把它关掉。

开关只管新挑的节点，已经跑着的出口不会因为改设置被换掉。

### Psiphon 出口

新建出口时把「出口来源」切到 **Psiphon**，再选地区和数量。地区清单先用内置的
28 个，任意一条 Psiphon 出口连上后换成 Psiphon 实时报上来的可用地区。
「不限地区」交给 Psiphon 挑最快的服务器，「每个国家」每个地区各开几条。

原理和 VPN Gate 出口不一样：每条 Psiphon 出口是一个独立的 `psiphon-tunnel-core`
进程，只在 `127.0.0.1` 上开一个本地 SOCKS 端口，不建 netns、不改路由。
母机上的混合端口收到连接后，转手经这个本地端口出去：

```
客户端 ──> 母机混合端口 :随机端口 ──> 127.0.0.1:psiphon 本地端口 ──> Psiphon 服务器
```

端口、口令、换节点、停止、重启后恢复都和 VPN Gate 出口一样用，有几点不同：

- **只转 TCP**。Psiphon 的本地代理不支持 UDP，SOCKS5 的 UDP ASSOCIATE 会直接回
  「命令不支持」，客户端会退回 TCP。要 UDP（DNS/QUIC 走隧道）请用 VPN Gate 出口。
- **出口 IP 可能变**。服务器由 Psiphon 在地区内自己挑，掉线后它会自动重连，
  可能换到同地区另一台。健康检查只看能不能出网，IP 变了就更新界面上的显示。
  重启 fanout 时会沿用这条出口自己的 datastore，Psiphon 会优先连回上次那台。
- **换节点**会清掉这条出口的 datastore，让 Psiphon 重新挑服务器；
  又分到换过的 IP 就再挑，最多 3 次。
- **是机房 IP**，不是家宽。「只用家宽节点」只管 VPN Gate，不影响 Psiphon。
- 域名交给 Psiphon 服务器解析，母机上不发 DNS 查询。
- 每条出口一个进程，内存按每条几十 MB 估算。

`psiphon-tunnel-core` 在第一次开 Psiphon 出口时自动下载：取自
[Psiphon-Labs/psiphon-tunnel-core-binaries](https://github.com/Psiphon-Labs/psiphon-tunnel-core-binaries)
钉死的提交，校验 SHA256 后放在 `/var/lib/fanout/bin/`，Docker 下随卷保留。
每条出口的配置、日志、通知和 datastore 在 `/var/lib/fanout/psiphon/<槽位>/`，
连不上时先看那里的 `notices.jsonl`。

官方只发布 **x86_64** 的 Linux 客户端。arm64 机器要用的话，自己编译
psiphon-tunnel-core 的 ConsoleClient，再用环境变量指过去：

```bash
# systemd：systemctl edit fanout，加上
[Service]
Environment=FANOUT_PSIPHON_BIN=/usr/local/bin/psiphon-tunnel-core

# Docker：docker run 加 -e FANOUT_PSIPHON_BIN=...，并把文件挂进容器
```

## 运维

装完后敲 `f` 打开管理菜单：启停、看日志、查隧道、看登录链接、改端口、卸载。

```
  状态      运行中
  版本      fanout v2.0.1
  开机自启  enabled

  登录链接  http://1.2.3.4:8899/?k=Zx9QpL2mVb7RkT4wYc8HnJ3sFd6GaE1u

   1) 启动          2) 停止
   3) 重启          4) 查看日志
   5) 隧道列表      6) 连接信息
   7) 改端口        8) 重置登录链接
   9) 开机自启开关
  10) 更新         11) 卸载
  12) 交流群 / 反馈
```

也可以直接带参数用：

```bash
f info       # 连接信息（含登录链接）
f link       # 只打印登录链接
f list       # 隧道列表
f restart    # 重启
f log        # 跟踪日志
f update     # 更新到最新版
f uninstall  # 卸载（含 netns、iptables 与 sysctl 配置）
```

隧道状态存在 `/var/lib/fanout/state.json`，重启后自动恢复，端口保持不变。

健康检查每 10 秒跑一次，比对出口 IP 是否还是建立隧道时那个——openvpn 挂掉后
netns 仍能经母机 NAT 出网，只看通不通会漏判。连续两次不符就自动换节点重连，
槽位和端口不变，原先指向它的客户端配置继续可用。

升级可以在界面里点「检查更新 → 更新到 vX.Y.Z」，也可以用 `f update`；
两处都是下载对应架构的发布包、替换二进制再重启服务，配置与隧道状态都保留。
也可以直接重跑一遍 install.sh。

## 已知限制

- 域名仍在本机解析：SOCKS5 的域名由 fanout 在母机上解析后再从隧道出去。
- VPN Gate 是志愿者节点，有相当比例已下线或满员（`AUTH_FAILED`）。
  启动时连不上会自动顺着同地区候选往下试，最多 6 个；失败原因会写在日志和界面上。
- 管理界面是明文 HTTP：登录链接和 cookie 在网络上能被截获。放公网建议前面套一层
  HTTPS 反代，或者在设置里把监听地址改成仅本机、用 SSH 隧道访问。
- 换节点会避开这条出口之前用过的节点（最多记 16 个），同地区都换过一轮后
  从头开始。一个地区只有一个可用节点时换不动，界面会直接说明原因。
- Psiphon 出口只转 TCP、出口 IP 不固定，官方客户端只有 x86_64，详见
  [Psiphon 出口](#psiphon-出口)。
- IP 质量来自 IPPure 的测试版接口，字段和可用性可能变化；风险分档是 fanout 自己定的，仅供参考。

## 许可

[MIT](LICENSE)。

节点来自 [VPN Gate](https://www.vpngate.net/)（筑波大学的学术实验项目），
本工具只是调用其公开的节点列表并用官方 openvpn 客户端连接，不修改也不代理其服务。
Psiphon 出口使用 Psiphon 官方发布的 psiphon-tunnel-core 客户端。
IP 质量数据来自 [IPPure](https://ippure.com/)；地图使用 [Natural Earth](https://www.naturalearthdata.com/)（公有领域）。
使用时请遵守 VPN Gate、Psiphon 的条款和你所在地的法律。

## 交流

- 交流群：<https://t.me/+ft-zI76oovgwNmRh>
- 视频教程：<https://youtube.com/@joeyblog>
- 博客：<https://joeyblog.net>

用着有问题、或者想要什么功能，去群里说或提 issue。