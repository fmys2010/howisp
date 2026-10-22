package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestAuth 在临时目录里建一套登录，返回包好的 handler：被放行的请求回 200 "ok"。
func newTestAuth(t *testing.T) (*Auth, http.Handler) {
	t.Helper()
	a, _, err := NewAuth(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	return a, h
}

func do(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func sessionFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

// 密钥第一次启动时生成、只给 root 读，之后原样读回；老版本的口令和访问路径文件要清掉。
func TestNewAuthCreatesAndReloadsKey(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"password", "basepath"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("old\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, created, err := NewAuth(dir)
	if err != nil || !created {
		t.Fatalf("第一次应当新建密钥: %v %v", created, err)
	}
	if len(a.Key()) != 32 {
		t.Fatalf("密钥长度不对: %q", a.Key())
	}
	info, err := os.Stat(filepath.Join(dir, keyFile))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("密钥文件权限应为 0600: %v %v", info, err)
	}
	for _, f := range []string{"password", "basepath"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Fatalf("老的 %s 应当被删掉", f)
		}
	}

	b, created, err := NewAuth(dir)
	if err != nil || created || b.Key() != a.Key() {
		t.Fatalf("第二次应当读回同一个密钥: %v %v", created, err)
	}
}

// 没登录时一切都是 404：页面、API、错的密钥，都不能看出这里跑着什么。
func TestUnauthenticatedIsNotFound(t *testing.T) {
	_, h := newTestAuth(t)
	for _, target := range []string{"/", "/api/exits", "/?k=wrong", "/login"} {
		rec := do(h, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s 应当 404，实际 %d", target, rec.Code)
		}
		if sessionFrom(rec) != nil {
			t.Fatalf("%s 不该发 cookie", target)
		}
	}
}

// 打开登录链接：发 30 天 cookie，跳回不带密钥的地址；之后凭 cookie 就能访问。
func TestLoginLinkIssuesCookie(t *testing.T) {
	a, h := newTestAuth(t)
	rec := do(h, httptest.NewRequest(http.MethodGet, "/?k="+a.Key()+"&tab=x", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("应当 303 跳转，实际 %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/?tab=x" {
		t.Fatalf("跳转地址要去掉密钥、保留别的参数，实际 %q", loc)
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("要带 Referrer-Policy: no-referrer")
	}
	c := sessionFrom(rec)
	if c == nil || !c.HttpOnly || c.MaxAge != int(sessionTTL.Seconds()) || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie 属性不对: %+v", c)
	}
	if strings.Contains(c.Value, a.Key()) {
		t.Fatal("cookie 里不能直接放密钥")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/exits", nil)
	r.AddCookie(c)
	if rec := do(h, r); rec.Code != http.StatusOK {
		t.Fatalf("带 cookie 应当放行，实际 %d", rec.Code)
	} else if sessionFrom(rec) != nil {
		t.Fatal("新 cookie 不用续签")
	}
}

// 篡改、过期、重置密钥之后的 cookie 都不能用。
func TestCookieRejected(t *testing.T) {
	a, h := newTestAuth(t)
	rec := do(h, httptest.NewRequest(http.MethodGet, "/?k="+a.Key(), nil))
	good := sessionFrom(rec)

	parts := strings.Split(good.Value, ".")
	later := strconv.FormatInt(time.Now().Add(400*24*time.Hour).Unix(), 10)
	expired := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	cases := map[string]string{
		"改了到期时间": later + "." + parts[1] + "." + parts[2],
		"过期":     expired + "." + parts[1] + "." + sign(a.Key(), mustInt(expired), parts[1]),
		"乱写":     "garbage",
		"签名不对":   parts[0] + "." + parts[1] + ".AAAA",
	}
	for name, v := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: v})
		if rec := do(h, r); rec.Code != http.StatusNotFound {
			t.Fatalf("%s 的 cookie 应当被拒，实际 %d", name, rec.Code)
		}
	}

	if _, err := a.ResetKey(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(good)
	if rec := do(h, r); rec.Code != http.StatusNotFound {
		t.Fatalf("重置密钥后旧 cookie 应当失效，实际 %d", rec.Code)
	}
}

func mustInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// 快到期的 cookie 自动续签，常用的设备一直不用重新登录。
func TestCookieRenewedNearExpiry(t *testing.T) {
	a, h := newTestAuth(t)
	exp := time.Now().Add(10 * 24 * time.Hour).Unix()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: strconv.FormatInt(exp, 10) + ".n." + sign(a.Key(), exp, "n")})
	rec := do(h, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("还没过期的 cookie 应当放行，实际 %d", rec.Code)
	}
	c := sessionFrom(rec)
	if c == nil || c.MaxAge != int(sessionTTL.Seconds()) {
		t.Fatalf("剩 10 天应当续签一张 30 天的新 cookie: %+v", c)
	}
}

// 命令行带请求头调 API 可以，但只限 API；错的密钥照样 404。
func TestKeyHeaderOnlyForAPI(t *testing.T) {
	a, h := newTestAuth(t)
	r := httptest.NewRequest(http.MethodGet, "/api/tunnels", nil)
	r.Header.Set(keyHeader, a.Key())
	if rec := do(h, r); rec.Code != http.StatusOK || sessionFrom(rec) != nil {
		t.Fatalf("请求头带密钥应当直接放行 API、不发 cookie，实际 %d", rec.Code)
	}

	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(keyHeader, a.Key())
	if rec := do(h, r); rec.Code != http.StatusNotFound {
		t.Fatalf("请求头只认 API，页面应当 404，实际 %d", rec.Code)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/tunnels", nil)
	r.Header.Set(keyHeader, "wrong")
	if rec := do(h, r); rec.Code != http.StatusNotFound {
		t.Fatalf("错的密钥应当 404，实际 %d", rec.Code)
	}
}

// 重置密钥的接口：换了密钥、当前浏览器拿到新 cookie，其他人的旧 cookie 失效。
func TestAPIKeyReset(t *testing.T) {
	a, _ := newTestAuth(t)
	old := a.Key()
	rec := httptest.NewRecorder()
	apiKeyReset(a)(rec, httptest.NewRequest(http.MethodPost, "/api/key/reset", nil))
	if rec.Code != http.StatusOK || a.Key() == old {
		t.Fatalf("应当换成新密钥: %d", rec.Code)
	}
	c := sessionFrom(rec)
	if c == nil {
		t.Fatal("重置的人要拿到新 cookie，不能把自己也踢出去")
	}
	if _, ok := a.cookieExpiry(c.Value); !ok {
		t.Fatal("新 cookie 应当按新密钥签名")
	}
	if !strings.Contains(rec.Body.String(), a.Key()) {
		t.Fatal("返回里应当带上新密钥")
	}

	rec = httptest.NewRecorder()
	apiKeyReset(a)(rec, httptest.NewRequest(http.MethodGet, "/api/key/reset", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 不该能重置，实际 %d", rec.Code)
	}
}

// 登录链接按设置里的端口和监听地址拼；只监听本机时就给本机地址。
func TestLoginLink(t *testing.T) {
	dir := t.TempDir()
	if _, err := loginLink(dir, "1.2.3.4"); err == nil {
		t.Fatal("还没有密钥时应当报错")
	}
	a, _, err := NewAuth(dir)
	if err != nil {
		t.Fatal(err)
	}
	link, err := loginLink(dir, "1.2.3.4")
	if err != nil || link != "http://1.2.3.4:8899/?k="+a.Key() {
		t.Fatalf("默认端口的链接不对: %q %v", link, err)
	}
	if err := os.WriteFile(webSettingsFilePath(dir), []byte(`{"port": 9000, "listen_addr": "127.0.0.1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link, _ = loginLink(dir, "1.2.3.4")
	if link != "http://127.0.0.1:9000/?k="+a.Key() {
		t.Fatalf("只监听本机时应当给本机地址: %q", link)
	}
}

// f 菜单跟着二进制走：是我们装的就换成配套版本，别人的同名文件不碰。
func TestSyncMenuScript(t *testing.T) {
	dir := t.TempDir()
	prev := menuScriptPath
	menuScriptPath = filepath.Join(dir, "f")
	defer func() { menuScriptPath = prev }()

	if err := os.WriteFile(menuScriptPath, []byte("#!/usr/bin/env bash\n# fanout 管理菜单\necho old\n"), 0755); err != nil {
		t.Fatal(err)
	}
	syncMenuScript()
	got, _ := os.ReadFile(menuScriptPath)
	if string(got) != strings.ReplaceAll(string(menuScript), "\r\n", "\n") {
		t.Fatal("旧菜单应当被换成配套版本")
	}
	if strings.Contains(string(got), "\r\n") {
		t.Fatal("写出去的脚本不能带 CRLF")
	}

	other := []byte("#!/bin/sh\necho someone else\n")
	if err := os.WriteFile(menuScriptPath, other, 0755); err != nil {
		t.Fatal(err)
	}
	syncMenuScript()
	if got, _ := os.ReadFile(menuScriptPath); string(got) != string(other) {
		t.Fatal("不是 fanout 的脚本不该被改")
	}
}
