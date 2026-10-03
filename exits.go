package main

import "time"

// Exit 是界面上的一行：一条隧道就是一个出口，对应一个 SOCKS5 端口。
type Exit struct {
	Slot    int       `json:"slot"`
	Port    int       `json:"port"` // SOCKS5 端口
	Host    string    `json:"host"`
	Region  string    `json:"region"`
	Country string    `json:"country"`
	ExitIP  string    `json:"exit_ip"`
	Status  string    `json:"status"`
	Err     string    `json:"err,omitempty"`
	Since   time.Time `json:"since"`
	// SOCKS5 凭据：界面要能看、能复制、能改
	SocksUser string `json:"socks_user"`
	SocksPass string `json:"socks_pass"`
}

// ExitsView 是主界面需要的全部数据。
type ExitsView struct {
	Exits []Exit `json:"exits"`
	// PublicIP 是母机公网 IPv4，前端用它拼 SOCKS5 地址
	PublicIP string `json:"public_ip"`
}

// ExitsOf 汇总当前所有出口。
func (m *Manager) ExitsOf() ExitsView {
	view := ExitsView{Exits: []Exit{}, PublicIP: hostPublicIP()}
	for _, v := range m.Views() {
		view.Exits = append(view.Exits, Exit{
			Slot:      v.Slot,
			Port:      v.Port,
			Host:      v.Node.HostName,
			Region:    v.Node.CountryCode,
			Country:   nodeLabel(v.Node),
			ExitIP:    v.ExitIP,
			Status:    v.Status,
			Err:       v.Err,
			Since:     v.Since,
			SocksUser: v.Cred.User,
			SocksPass: v.Cred.Pass,
		})
	}
	return view
}