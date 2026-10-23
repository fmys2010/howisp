package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 管理界面的登录：一条带密钥的链接 + 签名 cookie。
//
// 打开一次 http://IP:端口/?k=<密钥>，服务端发一个 30 天的签名 cookie，
// 再跳回不带密钥的地址（别让密钥留在地址栏和浏览记录里）。之后凭 cookie 访问，
// 重启、升级都不掉线；常用的设备会自动续期，一直不用重新登录。
//
// 没有 cookie 也没有密钥的请求一律 404：没有登录页、没有表单，
// 扫端口的看不出这里跑着什么，也无从爆破——密钥是 192 位随机数，猜不出来。
//
// cookie 用密钥本身做 HMAC，服务端不存会话：重置密钥，所有设备的 cookie 一起失效。
//
// 以前是"随机访问路径 + 口令"两样东西，会话只活 12 小时、重启就全掉，用起来太麻烦。

const (
	keyFile       = "key"
	sessionCookie = "fanout_session"
	sessionTTL    = 30 * 24 * time.Hour
	// sessionRenew 是剩余有效期低于多少时续签。常用的设备每隔半个月自动换一张新 cookie。
	sessionRenew = 15 * 24 * time.Hour
	// keyHeader 给 f 脚本这类命令行调用用：带着密钥直接调 /api/，不用走 cookie。
	keyHeader = "X-Fanout-Key"
)

// Auth 管登录密钥与 cookie 的签发、校验。
type Auth struct {
	dir string
	mu  sync.RWMutex
	key string
}

// NewAuth 载入或生成登录密钥。返回密钥是否为本次新建。
//
// 第一次生成密钥时顺手删掉老版本留下的 password / basepath：
// 那两样已经不起作用了，留着只会让人以为还能用。
func NewAuth(dir string) (*Auth, bool, error) {
	path := filepath.Join(dir, keyFile)
	blob, err := os.ReadFile(path)
	if err == nil {
		if key := strings.TrimSpace(string(blob)); key != "" {
			return &Auth{dir: dir, key: key}, false, nil
		}
	} else if !os.IsNotExist(err) {
		return nil, false, err
	}

	a := &Auth{dir: dir}
	if _, err := a.ResetKey(); err != nil {
		return nil, false, err
	}
	for _, legacy := range []string{"password", "basepath"} {
		if os.Remove(filepath.Join(dir, legacy)) == nil {
			log.Printf("登录方式已换成登录链接，删除不再使用的 %s", filepath.Join(dir, legacy))
		}
	}
	return a, true, nil
}

// newKey 生成一个登录密钥：24 字节随机数，URL 安全的 base64，32 个字符。
func newKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Key 返回当前密钥。
func (a *Auth) Key() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.key
}

// ResetKey 换一个新密钥并落盘。所有已发出的 cookie 随之失效。
func (a *Auth) ResetKey() (string, error) {
	key, err := newKey()
	if err != nil {
		return "", err
	}
	path := filepath.Join(a.dir, keyFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(key+"\n"), 0600); err != nil {
		return "", fmt.Errorf("写登录密钥失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("保存登录密钥失败: %w", err)
	}
	a.mu.Lock()
	a.key = key
	a.mu.Unlock()
	return key, nil
}

// checkKey 比对密钥。先各自哈希再比，长度不同也走同样的路径，不泄漏时序。
func (a *Auth) checkKey(k string) bool {
	cur := a.Key()
	if cur == "" || k == "" {
		return false
	}
	want := sha256.Sum256([]byte(cur))
	got := sha256.Sum256([]byte(strings.TrimSpace(k)))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

// sign 给 cookie 的有效期和随机数签名。
func sign(key string, exp int64, nonce string) string {
	m := hmac.New(sha256.New, []byte(key))
	fmt.Fprintf(m, "fanout-session/v1|%d|%s", exp, nonce)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// issueCookie 签发一张新 cookie：<到期时间>.<随机数>.<签名>。
func (a *Auth) issueCookie(w http.ResponseWriter) error {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	exp := time.Now().Add(sessionTTL).Unix()
	nonce := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    strconv.FormatInt(exp, 10) + "." + nonce + "." + sign(a.Key(), exp, nonce),
		Path:     "/",
		HttpOnly: true,
		// Lax 而不是 Strict：从聊天软件、书签点进来是跨站的顶层跳转，
		// Strict 不带 cookie，用户会对着一个 404 发愣
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

// cookieExpiry 校验 cookie，返回它的到期时间。
func (a *Auth) cookieExpiry(v string) (time.Time, bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	want := sign(a.Key(), exp, parts[1])
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) != 1 {
		return time.Time{}, false
	}
	at := time.Unix(exp, 0)
	if !time.Now().Before(at) {
		return time.Time{}, false
	}
	return at, true
}

// Wrap 保护整个管理界面。
func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 页面里有外链，别把管理界面的地址经 Referer 带出去
		w.Header().Set("Referrer-Policy", "no-referrer")

		// 命令行调用：请求头带密钥，只放行 API
		if k := r.Header.Get(keyHeader); k != "" {
			if strings.HasPrefix(r.URL.Path, "/api/") && a.checkKey(k) {
				next.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}

		// 登录链接：发 cookie，再跳回不带密钥的地址
		if k := r.URL.Query().Get("k"); k != "" {
			if !a.checkKey(k) {
				http.NotFound(w, r)
				return
			}
			if err := a.issueCookie(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			u := *r.URL
			q := u.Query()
			q.Del("k")
			u.RawQuery = q.Encode()
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}

		if c, err := r.Cookie(sessionCookie); err == nil {
			if exp, ok := a.cookieExpiry(c.Value); ok {
				if time.Until(exp) < sessionRenew {
					_ = a.issueCookie(w)
				}
				next.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
}
