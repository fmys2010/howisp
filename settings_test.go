package main

import (
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestNormalizeListenAddr(t *testing.T) {
	cases := map[string]string{
		"":          "",
		"0.0.0.0":   "",
		"all":       "",
		"127.0.0.1": "127.0.0.1",
	}
	for in, want := range cases {
		got, err := normalizeListenAddr(in)
		if err != nil {
			t.Fatalf("normalizeListenAddr(%q) 意外报错: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizeListenAddr(%q)=%q，想要 %q", in, got, want)
		}
	}
	if _, err := normalizeListenAddr("not-an-ip"); err == nil {
		t.Fatal("非法监听地址应当报错")
	}
}

func TestValidatePort(t *testing.T) {
	for _, p := range []int{1, 8899, 65535} {
		if err := validatePort(p); err != nil {
			t.Fatalf("端口 %d 应合法: %v", p, err)
		}
	}
	for _, p := range []int{0, -1, 70000} {
		if err := validatePort(p); err == nil {
			t.Fatalf("端口 %d 应非法", p)
		}
	}
}

func TestWebServerReloadSwitchesPort(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadWebSettings(dir, 0, false); err != nil {
		t.Fatalf("loadWebSettings: %v", err)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	srv := newWebServer(h)

	// 用两个系统分配的空闲端口验证切换
	p1 := freePort(t)
	if err := srv.reload(WebSettings{Port: p1, ListenAddr: "127.0.0.1"}); err != nil {
		t.Fatalf("reload p1: %v", err)
	}
	waitServe(t, p1)

	p2 := freePort(t)
	if err := srv.applyWebSettings(WebSettings{Port: p2, ListenAddr: "127.0.0.1"}); err != nil {
		t.Fatalf("applyWebSettings p2: %v", err)
	}
	waitServe(t, p2)

	// 旧端口应在优雅关闭后不再接受连接
	time.Sleep(1500 * time.Millisecond)
	if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p1)), 300*time.Millisecond); err == nil {
		c.Close()
		t.Fatalf("旧端口 %d 切换后仍在监听", p1)
	}

	// 非法端口应被拒，且不影响现有监听
	if err := srv.applyWebSettings(WebSettings{Port: 70000, ListenAddr: "127.0.0.1"}); err == nil {
		t.Fatal("非法端口应被拒")
	}
	waitServe(t, p2)
}

func waitServe(t *testing.T, port int) {
	t.Helper()
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	for i := 0; i < 40; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("端口 %d 未在预期时间内提供服务", port)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("取空闲端口失败: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// 界面上改过端口会落盘。之后再带 -web 启动时，命令行必须说话算话，
// 否则用户敲了参数却连不上，还没有任何提示（ct-54 上真实踩到）。
func TestLoadWebSettingsExplicitFlagWins(t *testing.T) {
	dir := t.TempDir()

	// 首次启动：建档存 8899
	if _, err := loadWebSettings(dir, 8899, false); err != nil {
		t.Fatalf("首次: %v", err)
	}

	// 不带 -web 重启：沿用盘上的 8899，不被默认值覆盖
	s, err := loadWebSettings(dir, 8899, false)
	if err != nil {
		t.Fatalf("沿用: %v", err)
	}
	if s.Port != 8899 {
		t.Fatalf("没显式指定时应沿用盘上的值，实际 %d", s.Port)
	}

	// 显式 -web 80：以命令行为准
	s, err = loadWebSettings(dir, 80, true)
	if err != nil {
		t.Fatalf("显式指定: %v", err)
	}
	if s.Port != 80 {
		t.Fatalf("显式 -web 应压过盘上的值，实际 %d", s.Port)
	}

	// 且要落盘，下次不带参数启动仍是 80
	s, err = loadWebSettings(dir, 8899, false)
	if err != nil {
		t.Fatalf("复读: %v", err)
	}
	if s.Port != 80 {
		t.Fatalf("显式指定的端口应已写回，实际 %d", s.Port)
	}
}
