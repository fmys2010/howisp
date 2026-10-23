package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 接口文档里经纬度是字符串、其余是数字；两种写法都要认，缺的字段要能看出来是缺的。
func TestParseIPInfo(t *testing.T) {
	body := `{"ip":"104.28.123.123","asn":13335,"asOrganization":"Cloudflare, Inc.",
	  "country":"United States","countryCode":"us","region":"California","city":"Los Angeles",
	  "longitude":"-118.24368","latitude":"34.05223","fraudScore":75,
	  "isResidential":false,"isBroadcast":true}`
	info, err := parseIPInfo([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if info.IP != "104.28.123.123" || info.ASN != 13335 || info.ASOrg != "Cloudflare, Inc." {
		t.Fatalf("基本字段不对: %+v", info)
	}
	if info.CountryCode != "US" {
		t.Fatalf("国家码应转成大写: %q", info.CountryCode)
	}
	if !info.HasGeo || info.Lat != 34.05223 || info.Lon != -118.24368 {
		t.Fatalf("字符串形式的经纬度没读出来: %+v", info)
	}
	if info.FraudScore != 75 || info.Residential || !info.Broadcast {
		t.Fatalf("质量字段不对: %+v", info)
	}

	// 经纬度是数字、没给风险分
	info, err = parseIPInfo([]byte(`{"ip":"1.2.3.4","latitude":35.6,"longitude":139.7}`))
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasGeo || info.Lat != 35.6 || info.FraudScore != -1 {
		t.Fatalf("数字经纬度或缺省风险分处理不对: %+v", info)
	}

	// 0,0 和越界都不算有位置，不然钉子会钉到几内亚湾
	for _, b := range []string{
		`{"ip":"1.2.3.4","latitude":"0","longitude":"0"}`,
		`{"ip":"1.2.3.4","latitude":"91","longitude":"10"}`,
		`{"ip":"1.2.3.4","latitude":"","longitude":"abc"}`,
	} {
		if info, err := parseIPInfo([]byte(b)); err != nil || info.HasGeo {
			t.Fatalf("%s 不该算有位置: %+v %v", b, info, err)
		}
	}

	for _, b := range []string{`not json`, `{"ip":""}`, `{"ip":"bogus"}`} {
		if _, err := parseIPInfo([]byte(b)); err == nil {
			t.Fatalf("%s 应当报错", b)
		}
	}
}

// 缓存的节奏：查到了 6 小时内不重查，查失败 2 分钟后再试，查询中不重复发起。
func TestIPInfoStoreClaim(t *testing.T) {
	var s ipinfoStore
	if !s.claim("1.1.1.1", false) {
		t.Fatal("没查过的 IP 应当去查")
	}
	if s.claim("1.1.1.1", false) || s.claim("1.1.1.1", true) {
		t.Fatal("正在查的不能重复发起，强制也不行")
	}
	s.finish("1.1.1.1", &IPInfo{IP: "1.1.1.1"}, nil)
	if s.get("1.1.1.1") == nil {
		t.Fatal("查到的结果应当能取到")
	}
	if s.claim("1.1.1.1", false) {
		t.Fatal("刚查到的不该重查")
	}
	if !s.claim("1.1.1.1", true) {
		t.Fatal("强制重查应当放行")
	}
	s.finish("1.1.1.1", nil, errors.New("boom"))
	if s.get("1.1.1.1") == nil {
		t.Fatal("重查失败不该把之前的结果丢掉")
	}

	if !s.claim("2.2.2.2", false) {
		t.Fatal("没查过的 IP 应当去查")
	}
	s.finish("2.2.2.2", nil, errors.New("timeout"))
	if s.claim("2.2.2.2", false) {
		t.Fatal("刚失败的要等冷却")
	}
	s.mu.Lock()
	s.m["2.2.2.2"].at = time.Now().Add(-ipinfoRetry - time.Second)
	s.mu.Unlock()
	if !s.claim("2.2.2.2", false) {
		t.Fatal("冷却过了应当重试")
	}
}

// 查的途中出口换了 IP：结果存到真实 IP 名下，原来那个 IP 记一次失败，别立刻重查。
func TestIPInfoStoreIPChanged(t *testing.T) {
	var s ipinfoStore
	s.claim("5.5.5.5", false)
	s.finish("5.5.5.5", &IPInfo{IP: "6.6.6.6"}, nil)
	if s.get("5.5.5.5") != nil {
		t.Fatal("别把 6.6.6.6 的结果挂到 5.5.5.5 上")
	}
	if got := s.get("6.6.6.6"); got == nil || got.IP != "6.6.6.6" {
		t.Fatalf("结果应当存到真实 IP 名下: %+v", got)
	}
	if s.claim("5.5.5.5", false) {
		t.Fatal("对不上的那个 IP 要等冷却再查")
	}
}

// 请求要从给定的出站方式发出去，这样 IPPure 看到的才是出口 IP。
func TestFetchIPInfoUsesGivenDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "fanout/") {
			t.Errorf("User-Agent 不对: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"9.9.9.9","asn":19281,"fraudScore":3,"latitude":"37.77","longitude":"-122.41"}`))
	}))
	defer srv.Close()
	prev := ipinfoURL
	ipinfoURL = srv.URL
	defer func() { ipinfoURL = prev }()

	dialed := 0
	info, err := fetchIPInfo(func(network, addr string) (net.Conn, error) {
		dialed++
		return net.Dial(network, addr)
	})
	if err != nil {
		t.Fatal(err)
	}
	if dialed == 0 {
		t.Fatal("没有走给定的出站方式")
	}
	if info.IP != "9.9.9.9" || info.FraudScore != 3 || !info.HasGeo {
		t.Fatalf("解析结果不对: %+v", info)
	}

	// 另起一个服务模拟限流；改正在跑的服务的 Handler 会被 -race 报竞争
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer limited.Close()
	ipinfoURL = limited.URL
	if _, err := fetchIPInfo(func(network, addr string) (net.Conn, error) { return net.Dial(network, addr) }); err == nil {
		t.Fatal("HTTP 429 应当报错")
	}
}

// 出口列表要带上查到的质量信息，母机那份单独给。
func TestExitsOfIncludesIPInfo(t *testing.T) {
	setPublicIPOverride("1.2.3.4")
	defer setPublicIPOverride("")

	m := NewManager(20, t.TempDir())
	m.tunnels[1] = &Tunnel{Slot: 1, node: psiphonNode("US"), status: "up", exitIP: "7.7.7.7"}
	m.tunnels[2] = &Tunnel{Slot: 2, node: psiphonNode("JP"), status: "starting"}
	m.ipinfo.claim("7.7.7.7", false)
	m.ipinfo.finish("7.7.7.7", &IPInfo{IP: "7.7.7.7", FraudScore: 10}, nil)
	m.ipinfo.claim(hostIPInfoKey, false)
	m.ipinfo.mu.Lock()
	m.ipinfo.m[hostIPInfoKey] = &ipinfoEntry{info: &IPInfo{IP: "1.2.3.4"}, at: time.Now()}
	m.ipinfo.mu.Unlock()

	v := m.ExitsOf()
	if v.Exits[0].IPInfo == nil || v.Exits[0].IPInfo.FraudScore != 10 {
		t.Fatalf("出口 1 应当带上质量信息: %+v", v.Exits[0])
	}
	if v.Exits[1].IPInfo != nil {
		t.Fatalf("还没出口 IP 的不该有质量信息: %+v", v.Exits[1])
	}
	if v.HostInfo == nil || v.HostInfo.IP != "1.2.3.4" {
		t.Fatalf("母机的质量信息没带上: %+v", v.HostInfo)
	}
}

// 页面里的地图占位要换成真正的 SVG，而且每个国家都带国家码，前端靠它定位。
func TestIndexPageEmbedsMap(t *testing.T) {
	if strings.Contains(indexPage, "<!--WORLDMAP-->") {
		t.Fatal("地图占位没被替换")
	}
	for _, want := range []string{`id="worldmap"`, `id="pins"`, `id="arcs"`, `data-cc="US"`, `data-cc="JP"`, `data-cc="FR"`} {
		if !strings.Contains(indexPage, want) {
			t.Fatalf("页面里缺少 %s", want)
		}
	}
}
