package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IP 质量与地理位置，来自 IPPure 的公开接口（https://ippure.com/MyIP-Info-API.html）。
//
// 这个接口报的是"调用者自己"的 IP，所以查哪个出口就得从哪个出口发请求：
// 经这条隧道的出站方式拨出去，IPPure 看到的就是出口 IP；母机自己那份从母机命名空间发。
//
// 结果按 IP 缓存，出口 IP 不变就不再查。接口还在测试阶段，没有公开的限额，
// 一台机器几十个出口一起轮询很容易被当成滥用，所以宁可少查。

// ipinfoURL 是变量而不是常量：测试里要换成本地假服务。
var ipinfoURL = "https://my.ippure.com/v1/info"

const (
	ipinfoTTL      = 6 * time.Hour    // 查到了多久后重查：IP 的风险评分会变，但不会变得很快
	ipinfoRetry    = 2 * time.Minute  // 查失败了多久后再试
	ipinfoInterval = 10 * time.Second // 多久扫一遍有没有新出口要查
	ipinfoWorkers  = 3                // 同时最多查几个
	ipinfoTimeout  = 20 * time.Second
)

// IPInfo 是一个 IP 的位置与质量。FraudScore 为 -1 表示接口没给。
type IPInfo struct {
	IP          string  `json:"ip"`
	ASN         int     `json:"asn"`
	ASOrg       string  `json:"as_org"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	HasGeo      bool    `json:"has_geo"`
	FraudScore  int     `json:"fraud_score"`
	Residential bool    `json:"residential"`
	// Broadcast 为 true 表示广播 IP（注册地和实际所在地不一致），false 是原生 IP
	Broadcast bool      `json:"broadcast"`
	Fetched   time.Time `json:"fetched"`
}

// flexFloat 兼容数字和字符串两种写法：接口文档里经纬度是字符串，别的字段是数字，
// 测试阶段的接口说变就变，两种都收。
type flexFloat struct {
	v  float64
	ok bool
}

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	f.v, f.ok = v, true
	return nil
}

// parseIPInfo 把接口返回解析成 IPInfo。没有合法 IP 的一律当失败。
func parseIPInfo(body []byte) (*IPInfo, error) {
	var raw struct {
		IP            string    `json:"ip"`
		ASN           flexFloat `json:"asn"`
		ASOrg         string    `json:"asOrganization"`
		Country       string    `json:"country"`
		CountryCode   string    `json:"countryCode"`
		Region        string    `json:"region"`
		City          string    `json:"city"`
		Longitude     flexFloat `json:"longitude"`
		Latitude      flexFloat `json:"latitude"`
		FraudScore    flexFloat `json:"fraudScore"`
		IsResidential bool      `json:"isResidential"`
		IsBroadcast   bool      `json:"isBroadcast"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("IPPure 返回的不是 JSON: %w", err)
	}
	ip := strings.TrimSpace(raw.IP)
	if net.ParseIP(ip) == nil {
		return nil, fmt.Errorf("IPPure 返回里没有合法 IP: %q", raw.IP)
	}
	info := &IPInfo{
		IP:          ip,
		ASN:         int(raw.ASN.v),
		ASOrg:       strings.TrimSpace(raw.ASOrg),
		Country:     strings.TrimSpace(raw.Country),
		CountryCode: strings.ToUpper(strings.TrimSpace(raw.CountryCode)),
		Region:      strings.TrimSpace(raw.Region),
		City:        strings.TrimSpace(raw.City),
		FraudScore:  -1,
		Residential: raw.IsResidential,
		Broadcast:   raw.IsBroadcast,
		Fetched:     time.Now(),
	}
	if raw.FraudScore.ok {
		info.FraudScore = int(math.Round(raw.FraudScore.v))
	}
	lat, lon := raw.Latitude, raw.Longitude
	if lat.ok && lon.ok && lat.v >= -90 && lat.v <= 90 && lon.v >= -180 && lon.v <= 180 &&
		!(lat.v == 0 && lon.v == 0) {
		info.Lat, info.Lon, info.HasGeo = lat.v, lon.v, true
	}
	return info, nil
}

// fetchIPInfo 经 dial 指定的出站方式问一次 IPPure。
func fetchIPInfo(dial dialFunc) (*IPInfo, error) {
	client := &http.Client{
		Timeout: ipinfoTimeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
				return dial(network, addr)
			},
			DisableKeepAlives:   true,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
	req, err := http.NewRequest(http.MethodGet, ipinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fanout/"+version)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询 IPPure 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询 IPPure 失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("查询 IPPure 失败: %w", err)
	}
	return parseIPInfo(body)
}

// mainNetnsDial 从母机命名空间拨号，查母机自己的 IP 用。
// 只走 IPv4：出口都是 IPv4，母机也按 IPv4 报，界面上才对得上。
func mainNetnsDial(network, addr string) (net.Conn, error) {
	var conn net.Conn
	err := inMainNetns(func() error {
		c, err := net.DialTimeout(forceIPv4Network(network), addr, 10*time.Second)
		conn = c
		return err
	})
	return conn, err
}

// ---- 缓存 ----

type ipinfoEntry struct {
	info    *IPInfo
	err     string
	at      time.Time // 上次查完（成功或失败）的时间
	pending bool
}

// due 判断这一项该不该（重新）查了。
func (e *ipinfoEntry) due(now time.Time) bool {
	if e == nil {
		return true
	}
	if e.pending {
		return false
	}
	if e.info != nil {
		return now.Sub(e.at) >= ipinfoTTL
	}
	return now.Sub(e.at) >= ipinfoRetry
}

// ipinfoStore 是按 IP 存的查询结果。零值可用。
type ipinfoStore struct {
	mu   sync.Mutex
	m    map[string]*ipinfoEntry
	sem  chan struct{}
	once sync.Once
}

func (s *ipinfoStore) init() {
	s.once.Do(func() {
		s.m = map[string]*ipinfoEntry{}
		s.sem = make(chan struct{}, ipinfoWorkers)
	})
}

// get 返回某个 IP 已查到的信息，没有就是 nil。
func (s *ipinfoStore) get(ip string) *IPInfo {
	if ip == "" {
		return nil
	}
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.m[ip]; e != nil {
		return e.info
	}
	return nil
}

// claim 判断 key 是否该查，该查就标成查询中并返回 true。force 无视缓存与失败冷却。
func (s *ipinfoStore) claim(key string, force bool) bool {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.m[key]
	if e != nil && e.pending {
		return false
	}
	if !force && !e.due(time.Now()) {
		return false
	}
	if e == nil {
		e = &ipinfoEntry{}
		s.m[key] = e
	}
	e.pending = true
	return true
}

// finish 记下一次查询的结果。
//
// 结果按接口报回来的 IP 存。和发起时以为的 IP（key）对不上，说明这期间出口换了
// （Psiphon 会自己换服务器）：结果照样存到真实 IP 名下，key 记一次失败，
// 免得下一轮又对着已经不存在的 IP 立刻重查。
func (s *ipinfoStore) finish(key string, info *IPInfo, err error) {
	s.init()
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.m[key]
	if e == nil {
		e = &ipinfoEntry{}
		s.m[key] = e
	}
	e.pending = false
	e.at = now
	switch {
	case err != nil:
		e.err = err.Error()
	case info.IP != key:
		e.err = "出口 IP 已变成 " + info.IP
		s.m[info.IP] = &ipinfoEntry{info: info, at: now}
	default:
		e.info, e.err = info, ""
	}
	// 只留最近用得着的：出口换来换去，旧 IP 的记录一直攒着没意义
	if len(s.m) > 512 {
		for ip, old := range s.m {
			if !old.pending && now.Sub(old.at) > ipinfoTTL {
				delete(s.m, ip)
			}
		}
	}
}

// lookup 在后台查 key，查完写回缓存。并发受 ipinfoWorkers 限制。
func (s *ipinfoStore) lookup(key string, dial dialFunc, force bool) {
	if !s.claim(key, force) {
		return
	}
	go func() {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		info, err := fetchIPInfo(dial)
		if err != nil {
			log.Printf("查询 %s 的 IP 质量失败: %v", key, err)
		}
		s.finish(key, info, err)
	}()
}

// ---- 管理器 ----

// hostIPInfoKey 是母机那份结果在缓存里的名字。母机 IP 可能被 -ip 覆盖成别的地址，
// 不能拿它当键去对 IPPure 报回来的 IP。
const hostIPInfoKey = "host"

// IPInfoOf 返回某个 IP 已查到的质量信息。
func (m *Manager) IPInfoOf(ip string) *IPInfo { return m.ipinfo.get(ip) }

// HostIPInfo 返回母机自己的质量信息。
func (m *Manager) HostIPInfo() *IPInfo {
	m.ipinfo.init()
	m.ipinfo.mu.Lock()
	defer m.ipinfo.mu.Unlock()
	if e := m.ipinfo.m[hostIPInfoKey]; e != nil {
		return e.info
	}
	return nil
}

// WatchIPInfo 周期查新出现的出口 IP（和母机自己）的质量。
func (m *Manager) WatchIPInfo() {
	for {
		m.scanIPInfo()
		time.Sleep(ipinfoInterval)
	}
}

func (m *Manager) scanIPInfo() {
	m.lookupHostIPInfo(false)
	for _, t := range m.Tunnels() {
		if t.statusOf() != "up" {
			continue
		}
		if ip := t.exitIPOf(); ip != "" {
			dial, _ := t.outbound()
			m.ipinfo.lookup(ip, dial, false)
		}
	}
}

// lookupHostIPInfo 查母机自己的 IP。和出口不同，母机那份不按 IP 存，
// 结果直接挂在 hostIPInfoKey 下。
func (m *Manager) lookupHostIPInfo(force bool) {
	s := &m.ipinfo
	if !s.claim(hostIPInfoKey, force) {
		return
	}
	go func() {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		info, err := fetchIPInfo(mainNetnsDial)
		if err != nil {
			log.Printf("查询母机的 IP 质量失败: %v", err)
		}
		s.mu.Lock()
		e := s.m[hostIPInfoKey]
		e.pending, e.at = false, time.Now()
		if err != nil {
			e.err = err.Error()
		} else {
			e.info, e.err = info, ""
		}
		s.mu.Unlock()
	}()
}

// RefreshIPInfo 立刻重查一个出口（slot 为 0 时是母机）的质量，不管缓存。
func (m *Manager) RefreshIPInfo(slot int) error {
	if slot == 0 {
		m.lookupHostIPInfo(true)
		return nil
	}
	m.mu.RLock()
	t, ok := m.tunnels[slot]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("槽位 %d 没有运行中的隧道", slot)
	}
	ip := t.exitIPOf()
	if t.statusOf() != "up" || ip == "" {
		return fmt.Errorf("这个出口还没连通，连上后会自动查")
	}
	dial, _ := t.outbound()
	m.ipinfo.lookup(ip, dial, true)
	return nil
}
