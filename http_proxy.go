package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// HTTP 代理，与 SOCKS5 共用同一个端口和同一套用户名/口令。
//
// 支持两种请求：
//   - CONNECT host:port   —— HTTPS 走这条，建立后原样转发
//   - GET http://...      —— 普通 HTTP，把请求行改写成源站形式再转发
//
// 认证走 Proxy-Authorization: Basic，失败回 407。

const proxyAuthRealm = "fanout"

// serveHTTP 处理一条 HTTP 代理连接。r 是已经缓冲了首字节的读取器。
func serveHTTP(client net.Conn, r *bufio.Reader, cred *SocksCred, dial dialFunc) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))

	req, err := http.ReadRequest(r)
	if err != nil {
		return
	}
	if !proxyAuthOK(req.Header.Get("Proxy-Authorization"), cred) {
		writeProxyAuthRequired(client)
		return
	}

	if req.Method == http.MethodConnect {
		serveConnect(client, r, req, dial)
		return
	}
	servePlainHTTP(client, r, req, dial)
}

// serveConnect 处理 CONNECT：连上目标后把这条连接变成隧道。
func serveConnect(client net.Conn, r *bufio.Reader, req *http.Request, dial dialFunc) {
	target := req.Host
	if target == "" {
		target = req.URL.Host
	}
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "443")
	}

	remote, err := dial("tcp", target)
	if err != nil {
		writeSimpleResponse(client, http.StatusBadGateway, "连接目标失败: "+err.Error())
		return
	}
	defer remote.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	// 隧道阶段不设超时，交给两端自然关闭
	_ = client.SetDeadline(time.Time{})
	_ = remote.SetDeadline(time.Time{})
	relay(client, r, remote)
}

// servePlainHTTP 处理绝对 URL 形式的普通 HTTP 请求。
//
// 只改写第一条请求行，之后双向转发：客户端后续发的本来就是源站形式，
// 原样透传即可，不用自己实现 keep-alive 的状态机。
func servePlainHTTP(client net.Conn, r *bufio.Reader, req *http.Request, dial dialFunc) {
	if !req.URL.IsAbs() || req.URL.Host == "" {
		writeSimpleResponse(client, http.StatusBadRequest, "这是代理端口，请发绝对 URL 或 CONNECT")
		return
	}
	target := req.URL.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "80")
	}

	remote, err := dial("tcp", target)
	if err != nil {
		writeSimpleResponse(client, http.StatusBadGateway, "连接目标失败: "+err.Error())
		return
	}
	defer remote.Close()

	// 只属于代理这一跳的头不能带给源站（凭据更不能）
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Proxy-Connection")
	// 没带 User-Agent 时不要凭空补一个 Go 的默认值，保持原样转发
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header["User-Agent"] = []string{""}
	}
	// 清掉绝对形式，Write 会写成源站形式的请求行
	req.RequestURI = ""
	req.URL.Scheme = ""
	req.URL.Host = ""
	req.URL.User = nil

	if err := req.Write(remote); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	_ = remote.SetDeadline(time.Time{})
	relay(client, r, remote)
}

// proxyAuthOK 校验 Proxy-Authorization: Basic。凭据为空时放行（内部调用路径不会走到）。
func proxyAuthOK(header string, cred *SocksCred) bool {
	if cred == nil || cred.User == "" {
		return true
	}
	const prefix = "Basic "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	// 恒定时间比较，和 SOCKS5 那套保持一致
	okUser := subtle.ConstantTimeCompare([]byte(user), []byte(cred.User)) == 1
	okPass := subtle.ConstantTimeCompare([]byte(pass), []byte(cred.Pass)) == 1
	return okUser && okPass
}

func writeProxyAuthRequired(w io.Writer) {
	body := "需要代理认证\n"
	_, _ = fmt.Fprintf(w, "HTTP/1.1 407 Proxy Authentication Required\r\n"+
		"Proxy-Authenticate: Basic realm=\"%s\"\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"Content-Length: %d\r\nConnection: close\r\n\r\n%s",
		proxyAuthRealm, len(body), body)
}

func writeSimpleResponse(w io.Writer, code int, msg string) {
	body := msg + "\n"
	_, _ = fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"Content-Length: %d\r\nConnection: close\r\n\r\n%s",
		code, http.StatusText(code), len(body), body)
}