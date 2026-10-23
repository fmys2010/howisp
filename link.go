package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// loginLink 拼出管理界面的登录链接：http://<地址>:<端口>/?k=<密钥>。
//
// 给 fanout -link 用，f 菜单、安装脚本、Docker 用户都从这里拿链接，
// 链接的格式只在这一处维护。只读不写：服务没在跑也能用。
func loginLink(dir, ipOverride string) (string, error) {
	blob, err := os.ReadFile(filepath.Join(dir, keyFile))
	if os.IsNotExist(err) {
		return "", fmt.Errorf("还没有登录密钥，先启动一次 fanout")
	}
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(blob))
	if key == "" {
		return "", fmt.Errorf("登录密钥文件是空的，删掉 %s 再重启 fanout", filepath.Join(dir, keyFile))
	}

	s := WebSettings{Port: 8899}
	if blob, err := os.ReadFile(webSettingsFilePath(dir)); err == nil {
		_ = json.Unmarshal(blob, &s)
		if s.Port == 0 {
			s.Port = 8899
		}
	}

	// 只监听某个具体地址时就用它（127.0.0.1 意味着要走 SSH 隧道）；
	// 监听所有网卡时用母机公网 IPv4
	host := strings.TrimSpace(s.ListenAddr)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = ipOverride
		if host == "" {
			host = os.Getenv("FANOUT_PUBLIC_IP")
		}
		if host == "" {
			host = probePublicIP()
		}
		if host == "" {
			host = "<本机IP>"
		}
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.Port)) + "/?k=" + url.QueryEscape(key), nil
}

// menuScript 是和这个二进制配套的 f 菜单。
//
//go:embed f.sh
var menuScript []byte

// menuScriptPath 是安装脚本放 f 菜单的位置。变量而不是常量：测试里要指到临时目录。
var menuScriptPath = "/usr/local/bin/f"

// syncMenuScript 把 f 菜单换成和这个二进制配套的版本。
//
// 两条升级路径（界面上的「更新」和 f update）都只换二进制，f 一直停在安装时那版。
// 平时问题不大，但登录方式换了之后，旧菜单会去读已经不存在的口令文件，
// 打印出来的地址也是错的。所以每次启动对一下，不一样就换掉。
// 只动看起来是 fanout 自己装的那个文件；别人放在那里的同名脚本不碰，Docker 里没有就跳过。
func syncMenuScript() {
	want := bytes.ReplaceAll(menuScript, []byte("\r\n"), []byte("\n"))
	cur, err := os.ReadFile(menuScriptPath)
	if err != nil || !bytes.Contains(cur, []byte("# fanout 管理菜单")) || bytes.Equal(cur, want) {
		return
	}
	tmp := menuScriptPath + ".tmp"
	if err := os.WriteFile(tmp, want, 0755); err != nil {
		log.Printf("更新 f 菜单失败: %v", err)
		return
	}
	if err := os.Rename(tmp, menuScriptPath); err != nil {
		_ = os.Remove(tmp)
		log.Printf("更新 f 菜单失败: %v", err)
		return
	}
	log.Printf("f 菜单已更新到与当前版本配套")
}
