package apply_test

import (
	"strings"
	"testing"

	"luban/internal/apply"
	"luban/internal/config"
)

func baseConfig() config.Config {
	return config.Config{
		System: config.System{Hostname: "router"},
		WAN: config.WAN{
			Mode:      "dhcp",
			Interface: "eth0",
			ModemIP:   "auto",
		},
		LAN: config.LAN{
			Interfaces: []string{"eth1", "eth2"},
			Address:    "192.168.20.1/24",
			DHCP: config.DHCP{
				Enabled: true,
				Start:   "192.168.20.100",
				End:     "192.168.20.254",
				Lease:   "12h",
			},
		},
		IPv6: config.IPv6{Enabled: false, LanPrefixLen: "auto"},
		DNS:  config.DNS{Upstreams: []string{"8.8.8.8"}},
	}
}

func TestBuildTemplateData_DHCP(t *testing.T) {
	cfg := baseConfig()
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if d.LanIface != "br0" {
		t.Errorf("LanIface = %q; want br0 (multiple ports)", d.LanIface)
	}
	if d.LanIP != "192.168.20.1" {
		t.Errorf("LanIP = %q; want 192.168.20.1", d.LanIP)
	}
	if d.MTU != 1500 {
		t.Errorf("MTU = %d; want 1500 for dhcp", d.MTU)
	}
	if !d.IsDHCP {
		t.Error("IsDHCP should be true")
	}
	if d.IsPPPoE || d.IsStatic {
		t.Error("IsPPPoE/IsStatic should be false for dhcp")
	}
}

func TestBuildTemplateData_PPPoE(t *testing.T) {
	cfg := baseConfig()
	cfg.WAN.Mode = "pppoe"
	cfg.WAN.PPPoE.Username = "user@isp"
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if d.MTU != 1492 {
		t.Errorf("MTU = %d; want 1492 for pppoe", d.MTU)
	}
	if !d.IsPPPoE {
		t.Error("IsPPPoE should be true")
	}
}

func TestBuildTemplateData_Bridge(t *testing.T) {
	cfg := baseConfig()
	cfg.WAN.Mode = "bridge"
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if !d.IsBridge {
		t.Error("IsBridge should be true for wan.mode=bridge")
	}
	if d.IsDHCP || d.IsStatic || d.IsPPPoE {
		t.Error("IsDHCP/IsStatic/IsPPPoE should all be false for wan.mode=bridge")
	}
	// MTU falls back to the 1500 default (bridge is not a routed link, but
	// the auto-MTU switch in BuildTemplateData has no bridge-specific case,
	// which is correct: wan.mtu can still override the bridged port's MTU).
	if d.MTU != 1500 {
		t.Errorf("MTU = %d; want 1500 (default) for bridge", d.MTU)
	}
}

func TestBuildTemplateData_SingleLAN(t *testing.T) {
	cfg := baseConfig()
	cfg.LAN.Interfaces = []string{"eth1"}
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if d.LanIface != "eth1" {
		t.Errorf("LanIface = %q; want eth1 for single port", d.LanIface)
	}
}

func TestBuildTemplateData_NoLAN(t *testing.T) {
	cfg := baseConfig()
	cfg.LAN.Interfaces = nil
	_, err := apply.BuildTemplateData(cfg)
	if err == nil {
		t.Error("expected error for empty lan.interfaces")
	}
}

func TestRenderOne_Fixture(t *testing.T) {
	cfg := baseConfig()
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatal(err)
	}

	tpl := `hostname={{ .System.Hostname }} iface={{ .LanIface }} ip={{ .LanIP }} mtu={{ .MTU }}`
	out, err := apply.RenderOne(tpl, d)
	if err != nil {
		t.Fatalf("RenderOne: %v", err)
	}

	if !strings.Contains(out, "hostname=router") {
		t.Errorf("missing hostname: %s", out)
	}
	if !strings.Contains(out, "iface=br0") {
		t.Errorf("missing iface: %s", out)
	}
	if !strings.Contains(out, "ip=192.168.20.1") {
		t.Errorf("missing ip: %s", out)
	}
	if !strings.Contains(out, "mtu=1500") {
		t.Errorf("missing mtu: %s", out)
	}
}

func TestBuildTemplateData_DnsUpstreamLines(t *testing.T) {
	cfg := baseConfig()
	cfg.DNS.Upstreams = []string{
		"119.29.29.29",
		"223.5.5.5:5353",
		"tcp://8.8.8.8:53",
		"tls://dns.google:853",
		"https://dns.alidns.com/dns-query",
		"quic://dns.adguard.com:853",
		"h3://cloudflare-dns.com/dns-query",
	}
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	want := []string{
		"server 119.29.29.29",
		"server 223.5.5.5:5353",
		"server-tcp 8.8.8.8:53",
		"server-tls dns.google:853",
		"server-https https://dns.alidns.com/dns-query",
		"server-quic dns.adguard.com:853",
		"server-h3 h3://cloudflare-dns.com/dns-query",
	}
	if len(d.DnsUpstreamLines) != len(want) {
		t.Fatalf("DnsUpstreamLines = %v; want %v", d.DnsUpstreamLines, want)
	}
	for i, line := range want {
		if d.DnsUpstreamLines[i] != line {
			t.Errorf("DnsUpstreamLines[%d] = %q; want %q", i, d.DnsUpstreamLines[i], line)
		}
	}
}

func TestBuildTemplateData_InvalidDnsUpstream(t *testing.T) {
	cfg := baseConfig()
	cfg.DNS.Upstreams = []string{"ftp://not-supported"}
	if _, err := apply.BuildTemplateData(cfg); err == nil {
		t.Error("expected error for unsupported dns upstream scheme")
	}
}

func TestBuildTemplateData_MTUOverride(t *testing.T) {
	cfg := baseConfig()
	cfg.WAN.MTU = 1400
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if d.MTU != 1400 {
		t.Errorf("MTU = %d; want 1400 (override)", d.MTU)
	}
}

func TestBuildTemplateData_MTUOverride_PPPoE(t *testing.T) {
	cfg := baseConfig()
	cfg.WAN.Mode = "pppoe"
	cfg.WAN.PPPoE.Username = "user@isp"
	cfg.WAN.MTU = 1400
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if d.MTU != 1400 {
		t.Errorf("MTU = %d; want 1400 (override wins over PPPoE default 1492)", d.MTU)
	}
}

func TestBuildTemplateData_MTUAuto(t *testing.T) {
	cfg := baseConfig()
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if d.MTU != 1500 {
		t.Errorf("MTU = %d; want 1500 (auto, dhcp)", d.MTU)
	}
}

func TestBuildTemplateData_MSSClampLine(t *testing.T) {
	cases := []struct {
		name string
		mss  int
		want string
	}{
		{name: "auto", mss: 0, want: "rt mtu"},
		{name: "manual", mss: 1360, want: "1360"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.WAN.MSS = tc.mss
			d, err := apply.BuildTemplateData(cfg)
			if err != nil {
				t.Fatalf("BuildTemplateData: %v", err)
			}
			if d.MSSClampLine != tc.want {
				t.Errorf("MSSClampLine = %q; want %q", d.MSSClampLine, tc.want)
			}
		})
	}
}

func TestBuildTemplateData_DhcpDNSServers_Auto(t *testing.T) {
	cfg := baseConfig()
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	want := []string{"192.168.20.1"}
	if len(d.DhcpDNSServers) != 1 || d.DhcpDNSServers[0] != want[0] {
		t.Errorf("DhcpDNSServers = %v; want %v (auto → self)", d.DhcpDNSServers, want)
	}
}

func TestBuildTemplateData_DhcpDNSServers_Manual(t *testing.T) {
	cfg := baseConfig()
	cfg.LAN.DHCP.DNSMode = "manual"
	cfg.LAN.DHCP.DNSServers = []string{"1.1.1.1", "8.8.8.8"}
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	want := []string{"1.1.1.1", "8.8.8.8"}
	if len(d.DhcpDNSServers) != len(want) {
		t.Fatalf("DhcpDNSServers = %v; want %v", d.DhcpDNSServers, want)
	}
	for i := range want {
		if d.DhcpDNSServers[i] != want[i] {
			t.Errorf("DhcpDNSServers[%d] = %q; want %q", i, d.DhcpDNSServers[i], want[i])
		}
	}
}

func TestRenderOne_JoinFunc(t *testing.T) {
	cfg := baseConfig()
	cfg.LAN.DHCP.DNSMode = "manual"
	cfg.LAN.DHCP.DNSServers = []string{"1.1.1.1", "8.8.8.8"}
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tpl := `dns={{ join .DhcpDNSServers "," }}`
	out, err := apply.RenderOne(tpl, d)
	if err != nil {
		t.Fatalf("RenderOne: %v", err)
	}
	if out != "dns=1.1.1.1,8.8.8.8" {
		t.Errorf("RenderOne join output = %q", out)
	}
}

func TestRenderOne_ConditionalBlock(t *testing.T) {
	cfg := baseConfig()
	cfg.WAN.Mode = "pppoe"
	cfg.WAN.PPPoE.Username = "isp-user"
	d, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatal(err)
	}

	tpl := `{{ if .IsPPPoE }}plugin rp-pppoe.so {{ .WAN.Interface }}{{ end }}`
	out, err := apply.RenderOne(tpl, d)
	if err != nil {
		t.Fatalf("RenderOne: %v", err)
	}
	if !strings.Contains(out, "plugin rp-pppoe.so") {
		t.Errorf("PPPoE block not rendered: %s", out)
	}
}
