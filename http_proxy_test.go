package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// 混合端口收到 CONNECT 时，认证通过就把连接变成隧道。
func TestHTTPProxyConnectTunnels(t *testing.T) {
	cred := &SocksCred{User: "u", Pass: "p"}
	remoteClient, remoteServer := net.Pipe()
	defer remoteServer.Close()

	dial := func(network, addr string) (net.Conn, error) {
		if addr != "example.com:443" {
			t.Errorf("拨号目标不对: %s", addr)
		}
		return remoteClient, nil
	}
	client, server := net.Pipe()
	defer client.Close()
	go serveMixed(server, cred, dial)
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))

	go func() {
		fmt.Fprintf(client, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: %s\r\n\r\n",
			basicAuth("u", "p"))
	}()

	br := bufio.NewReader(client)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT 应回 200，得到 %d", resp.StatusCode)
	}

	// 客户端 -> 目标
	go func() { _, _ = client.Write([]byte("ping")) }()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(remoteServer, buf); err != nil {
		t.Fatalf("目标没收到数据: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("转发内容不对: %q", buf)
	}

	// 目标 -> 客户端
	go func() { _, _ = remoteServer.Write([]byte("pong")) }()
	if _, err := io.ReadFull(br, buf); err != nil {
		t.Fatalf("客户端没收到回程数据: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("回程内容不对: %q", buf)
	}
}

// 没带凭据时回 407，并且不去拨号。
func TestHTTPProxyRejectsWithoutAuth(t *testing.T) {
	cred := &SocksCred{User: "u", Pass: "p"}
	dial := func(network, addr string) (net.Conn, error) {
		t.Error("认证没过就不该拨号")
		return nil, io.EOF
	}
	client, server := net.Pipe()
	defer client.Close()
	go serveMixed(server, cred, dial)
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))

	go func() {
		io.WriteString(client, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n")
	}()

	br := bufio.NewReader(client)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("应回 407，得到 %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Proxy-Authenticate"), "Basic") {
		t.Fatal("407 应当带上 Proxy-Authenticate: Basic")
	}
}

// 普通 HTTP 请求要改写成源站形式，并且不能把代理凭据带给源站。
func TestHTTPProxyPlainRequestRewritten(t *testing.T) {
	cred := &SocksCred{User: "u", Pass: "p"}
	remoteClient, remoteServer := net.Pipe()
	defer remoteServer.Close()

	dial := func(network, addr string) (net.Conn, error) {
		if addr != "example.com:80" {
			t.Errorf("拨号目标不对: %s", addr)
		}
		return remoteClient, nil
	}
	client, server := net.Pipe()
	defer client.Close()
	go serveMixed(server, cred, dial)
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))

	go func() {
		fmt.Fprintf(client, "GET http://example.com/path?q=1 HTTP/1.1\r\n"+
			"Host: example.com\r\nUser-Agent: test/1\r\nProxy-Authorization: %s\r\n\r\n",
			basicAuth("u", "p"))
	}()

	br := bufio.NewReader(remoteServer)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("源站没收到请求行: %v", err)
	}
	if got := strings.TrimSpace(line); got != "GET /path?q=1 HTTP/1.1" {
		t.Fatalf("请求行该是源站形式，得到 %q", got)
	}
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("读头失败: %v", err)
		}
		if strings.TrimSpace(h) == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(h), "proxy-") {
			t.Fatalf("代理专用的头不该转发给源站: %q", strings.TrimSpace(h))
		}
	}
}

func TestProxyAuthOK(t *testing.T) {
	cred := &SocksCred{User: "alice", Pass: "s3cret"}
	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"正确", basicAuth("alice", "s3cret"), true},
		{"basic 大小写不敏感", "basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret")), true},
		{"口令错", basicAuth("alice", "nope"), false},
		{"用户名错", basicAuth("bob", "s3cret"), false},
		{"空", "", false},
		{"没有 Basic 前缀", base64.StdEncoding.EncodeToString([]byte("alice:s3cret")), false},
		{"不是 base64", "Basic !!!not-base64!!!", false},
		{"没有冒号", "Basic " + base64.StdEncoding.EncodeToString([]byte("alices3cret")), false},
	}
	for _, c := range cases {
		if got := proxyAuthOK(c.header, cred); got != c.want {
			t.Errorf("%s: proxyAuthOK=%v，想要 %v", c.name, got, c.want)
		}
	}
	// 没设凭据时放行（内部路径不会走到，但不能反过来把人挡住）
	if !proxyAuthOK("", &SocksCred{}) {
		t.Error("没有凭据时应当放行")
	}
}

// 既不是 SOCKS5 也不是 HTTP 的首字节，直接断开，不要挂在那里。
func TestServeMixedRejectsUnknownProtocol(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go serveMixed(server, &SocksCred{User: "u", Pass: "p"}, func(network, addr string) (net.Conn, error) {
		return nil, io.EOF
	})
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))

	go func() { _, _ = client.Write([]byte{0x00, 0x01, 0x02}) }()
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("未知协议应当被直接断开")
	}
}