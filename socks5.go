package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// SOCKS5 实现：支持 CONNECT（TCP）与 UDP ASSOCIATE（UDP 中继，见 udp.go）。
//
// 监听在混合端口上，HTTP 代理见 http_proxy.go，两者共用同一套用户名/口令。
// 认证走 RFC1929。端口对公网敞开，没有口令等于谁扫到谁就能用这条家宽出口，
// 所以凭据是必需的而不是可选项。

const (
	socksVer5       = 0x05
	authNone        = 0x00
	authUserPass    = 0x02
	authNoAccept    = 0xff
	authSubVer      = 0x01
	cmdConnect      = 0x01
	cmdUDPAssociate = 0x03
	atypIPv4        = 0x01
	atypDomain      = 0x03
	atypIPv6        = 0x04
	repSuccess      = 0x00
	repGenFail      = 0x01
	repHostUnre     = 0x04
	repCmdNotSupp   = 0x07
)

// serveSocks 处理一条 SOCKS5 连接。
//
// r 是已经缓冲了首字节的读取器：握手和转发都必须从它读，直接读裸 conn
// 会把缓冲区里已有的数据丢掉（混合端口预读的那一个字节就属于这种情况）。
func serveSocks(client net.Conn, r io.Reader, cred *SocksCred, dial dialFunc) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))

	if err := socksHandshake(client, r, cred); err != nil {
		return
	}

	cmd, addr, err := socksReadRequestWithCmd(r)
	if err != nil {
		if errors.Is(err, errCmdNotSupported) {
			socksReply(client, repCmdNotSupp)
		} else {
			socksReply(client, repGenFail)
		}
		return
	}

	if cmd == cmdUDPAssociate {
		_ = client.SetDeadline(time.Time{})
		serveUDPAssociate(client, dial)
		return
	}

	remote, err := dial("tcp", addr)
	if err != nil {
		socksReply(client, repHostUnre)
		return
	}
	defer remote.Close()

	if err := socksReply(client, repSuccess); err != nil {
		return
	}

	// 转发阶段不设整体超时，交给两端自然关闭
	_ = client.SetDeadline(time.Time{})
	_ = remote.SetDeadline(time.Time{})
	relay(client, r, remote)
}

// socksHandshake 完成方法协商，需要认证时接着跑一轮 RFC1929。
func socksHandshake(c net.Conn, r io.Reader, cred *SocksCred) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(r, head); err != nil {
		return err
	}
	if head[0] != socksVer5 {
		return errors.New("不是 socks5")
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(r, methods); err != nil {
		return err
	}

	if cred == nil || cred.User == "" {
		_, err := c.Write([]byte{socksVer5, authNone})
		return err
	}

	// 客户端没提用户名口令就直接拒绝，不退回无认证
	if !bytes.ContainsRune(methods, rune(authUserPass)) {
		_, _ = c.Write([]byte{socksVer5, authNoAccept})
		return errors.New("客户端不支持用户名口令认证")
	}
	if _, err := c.Write([]byte{socksVer5, authUserPass}); err != nil {
		return err
	}
	return socksAuth(c, r, cred)
}

// socksAuth 跑一轮 RFC1929 用户名/口令子协商。
func socksAuth(c net.Conn, r io.Reader, cred *SocksCred) error {
	ver := make([]byte, 1)
	if _, err := io.ReadFull(r, ver); err != nil {
		return err
	}
	if ver[0] != authSubVer {
		return errors.New("认证子协议版本不对")
	}
	user, err := readLenPrefixed(r)
	if err != nil {
		return err
	}
	pass, err := readLenPrefixed(r)
	if err != nil {
		return err
	}

	// 恒定时间比较，避免按字节比对泄漏口令长度与前缀
	okUser := subtle.ConstantTimeCompare(user, []byte(cred.User)) == 1
	okPass := subtle.ConstantTimeCompare(pass, []byte(cred.Pass)) == 1
	if !okUser || !okPass {
		_, _ = c.Write([]byte{authSubVer, 0x01})
		return errors.New("用户名或口令不对")
	}
	_, err = c.Write([]byte{authSubVer, 0x00})
	return err
}

// readLenPrefixed 读一个单字节长度前缀的字段。
func readLenPrefixed(r io.Reader) ([]byte, error) {
	l := make([]byte, 1)
	if _, err := io.ReadFull(r, l); err != nil {
		return nil, err
	}
	b := make([]byte, int(l[0]))
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

var errCmdNotSupported = errors.New("仅支持 CONNECT 与 UDP ASSOCIATE")

// errIPv6NotSupported 表示拒绝 IPv6 目标：隧道内只有 IPv4。
var errIPv6NotSupported = errors.New("隧道内不支持 IPv6")

// socksReadRequest 读请求并返回目标地址（仅 CONNECT，兼容旧调用）。
func socksReadRequest(r io.Reader) (string, error) {
	cmd, addr, err := socksReadRequestWithCmd(r)
	if err != nil {
		return "", err
	}
	if cmd != cmdConnect {
		return "", errCmdNotSupported
	}
	return addr, nil
}

// socksReadRequestWithCmd 读请求并返回命令与目标地址，支持 CONNECT 与 UDP ASSOCIATE。
func socksReadRequestWithCmd(r io.Reader) (byte, string, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return 0, "", err
	}
	switch head[1] {
	case cmdConnect, cmdUDPAssociate:
	default:
		return 0, "", errCmdNotSupported
	}

	var host string
	switch head[3] {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(r, b); err != nil {
			return 0, "", err
		}
		host = net.IP(b).String()
	case atypIPv6:
		// 隧道内没有 IPv6 路由，放行只会让这条连接绕开隧道
		b := make([]byte, 16)
		if _, err := io.ReadFull(r, b); err != nil {
			return 0, "", err
		}
		return 0, "", errIPv6NotSupported
	case atypDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(r, l); err != nil {
			return 0, "", err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(r, b); err != nil {
			return 0, "", err
		}
		host = string(b)
	default:
		return 0, "", fmt.Errorf("不支持的地址类型 %d", head[3])
	}

	pb := make([]byte, 2)
	if _, err := io.ReadFull(r, pb); err != nil {
		return 0, "", err
	}
	return head[1], net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb)))), nil
}

func socksReply(c net.Conn, code byte) error {
	_, err := c.Write([]byte{socksVer5, code, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

// socksReplyWith 回一个带 BND 地址/端口的应答（UDP ASSOCIATE 需要真实中继地址）。
func socksReplyWith(c net.Conn, code byte, ip net.IP, port int) error {
	if ip == nil || ip.To4() == nil {
		ip = net.IPv4zero
	}
	b := make([]byte, 0, 10)
	b = append(b, socksVer5, code, 0x00, atypIPv4)
	b = append(b, ip.To4()...)
	b = append(b, byte(port>>8), byte(port))
	_, err := c.Write(b)
	return err
}

// relay 双向转发，任一方向结束就返回（调用方负责关连接）。
//
// clientR 是读客户端那一侧用的读取器：混合端口下它可能带着预读的数据，
// 所以不能直接用 client 读。
func relay(client net.Conn, clientR io.Reader, remote net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, clientR); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, remote); done <- struct{}{} }()
	<-done
}