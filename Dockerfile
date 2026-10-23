# fanout 容器镜像。
#
# 这个镜像不是普通的应用容器：fanout 要建 network namespace、改 iptables、
# 在 netns 里跑 openvpn，所以运行时必须是 host 网络 + NET_ADMIN/SYS_ADMIN，
# 并且能看到 /dev/net/tun。具体跑法见 README 与 docker-compose.yml。
#
# Psiphon 出口用的 psiphon-tunnel-core 不打进镜像：第一次开 Psiphon 出口时
# fanout 自己下载（钉死版本、校验 sha256）到 /var/lib/fanout/bin，跟着卷走，
# 版本只在 psiphon.go 一处维护。官方只发 x86_64，arm64 镜像里开不了 Psiphon 出口。

# ---------- 构建阶段 ----------
FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# 版本号由 CI 传进来（git tag），本地构建默认 dev
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/fanout .

# ---------- 运行阶段 ----------
FROM debian:bookworm-slim

# openvpn 连节点、curl 探出口 IP、iproute2 建 netns/veth、iptables 配 NAT 与转发
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      openvpn curl iproute2 iptables ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/fanout /usr/local/bin/fanout

# 状态和登录密钥都放这里，挂出来才能在重建容器后接着用
VOLUME /var/lib/fanout
EXPOSE 8899

ENTRYPOINT ["/usr/local/bin/fanout"]
CMD ["-dir", "/var/lib/fanout"]