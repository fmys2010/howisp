package main

import "time"

// Exit 是界面上的一行：一条隧道就是一个出口，对应一个 SOCKS5 端口。
type Exit struct {
	// Provider 是出口来源："vpngate"、"psiphon" 或 "warp"
	Provider string    `json:"provider"`
	Slot     int       `json:"slot"`
	Port     int       `json:"port"` // SOCKS5 端口
	Host     string    `json:"host"`
	Region   string    `json:"region"`
	Country  string    `json:"country"`
	ExitIP   string    `json:"exit_ip"`
	Status   string    `json:"status"`
	Err      string    `json:"err,omitempty"`
	Since    time.Time `json:"since"`
	// SOCKS5 凭据：界面要能看、能复制、能改
	SocksUser string `json:"socks_user"`
	SocksPass string `json:"socks_pass"`
	// IPInfo 是出口 IP 的位置与质量，还没查到时为空
	IPInfo *IPInfo `json:"ipinfo,omitempty"`
}

// ExitsView 是主界面需要的全部数据。
type ExitsView struct {
	Exits []Exit `json:"exits"`
	// PublicIP 是母机公网 IPv4，前端用它拼 SOCKS5 地址
	PublicIP string `json:"public_ip"`
	// HostInfo 是母机自己的位置与质量，地图上画连线的起点
	HostInfo *IPInfo `json:"host_info,omitempty"`
}

// ExitsOf 汇总当前所有出口。
func (m *Manager) ExitsOf() ExitsView {
	view := ExitsView{Exits: []Exit{}, PublicIP: hostPublicIP(), HostInfo: m.HostIPInfo()}
	for _, v := range m.Views() {
		provider, country := "vpngate", nodeLabel(v.Node)
		switch {
		case v.Node.isPsiphon():
			provider, country = providerPsiphon, psiphonRegionText(v.Node)
		case v.Node.isWarp():
			provider, country = providerWarp, warpLabel
		}
		view.Exits = append(view.Exits, Exit{
			Provider:  provider,
			Slot:      v.Slot,
			Port:      v.Port,
			Host:      v.Node.HostName,
			Region:    v.Node.CountryCode,
			Country:   country,
			ExitIP:    v.ExitIP,
			Status:    v.Status,
			Err:       v.Err,
			Since:     v.Since,
			SocksUser: v.Cred.User,
			SocksPass: v.Cred.Pass,
			IPInfo:    m.IPInfoOf(v.ExitIP),
		})
	}
	return view
}
