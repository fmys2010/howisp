package main

import (
	"bufio"
	"net"
)

// 混合端口：同一个端口既收 SOCKS5 也收 HTTP 代理请求。
//
// 判别只看第一个字节：SOCKS5 的版本号是 0x05，HTTP 的方法名一定以大写字母
// 开头。两种协议在首字节就分得清，不需要预读更多。
//
// 预读用 bufio，并且把同一个 reader 一路传下去：缓冲里可能已经带上了客户端
// 后续发的数据（比如 CONNECT 之后紧跟着的 TLS 握手），换回裸 conn 读就会丢。

// dialFunc 是按指定网络与地址建连的函数，由隧道注入（出站在 netns 里）。
type dialFunc func(network, addr string) (net.Conn, error)

// serveMixed 是混合端口的入口，按首字节把连接分给 SOCKS5 或 HTTP 代理。
func serveMixed(client net.Conn, cred *SocksCred, dial dialFunc) {
	br := bufio.NewReader(client)
	head, err := br.Peek(1)
	if err != nil {
		_ = client.Close()
		return
	}
	switch {
	case head[0] == socksVer5:
		serveSocks(client, br, cred, dial)
	case head[0] >= 'A' && head[0] <= 'Z':
		serveHTTP(client, br, cred, dial)
	default:
		// 既不是 SOCKS5 也不是 HTTP，直接断开，别占着隧道
		_ = client.Close()
	}
}