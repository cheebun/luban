package config_test

import (
	"luban/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_DefaultCreated(t *testing.T) {
	dir := t.TempDir()
	s, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	cfg := s.Get()
	if cfg.WAN.Mode != "dhcp" {
		t.Errorf("default wan.mode = %q; want dhcp", cfg.WAN.Mode)
	}
	if cfg.LAN.Address != "192.168.20.1/24" {
		t.Errorf("default lan.address = %q; want 192.168.20.1/24", cfg.LAN.Address)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json not written: %v", err)
	}
}

func TestNewStore_LoadExisting(t *testing.T) {
	dir := t.TempDir()
	raw := `{
		"system":{"hostname":"myrouter","admin":{"password_hash":"$2a$10$abc","must_change":false}},
		"wan":{"mode":"static","interface":"eth0","static":{"address":"1.2.3.4/24","gateway":"1.2.3.1","dns":[]},"pppoe":{},"modem_ip":"auto"},
		"lan":{"interfaces":["eth1"],"address":"192.168.1.1/24","dhcp":{"enabled":true,"start":"192.168.1.100","end":"192.168.1.200","lease":"12h"}},
		"ipv6":{"enabled":false,"lan_prefix_len":"auto"},
		"dns":{"upstreams":[]}
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o640); err != nil { //nolint:gosec // G306: matching production 0640 permission in test fixture
		t.Fatal(err)
	}
	s, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	cfg := s.Get()
	if cfg.System.Hostname != "myrouter" {
		t.Errorf("hostname = %q; want myrouter", cfg.System.Hostname)
	}
	if cfg.WAN.Mode != "static" {
		t.Errorf("wan.mode = %q; want static", cfg.WAN.Mode)
	}
}

func TestNewStore_LoadExisting_MissingDNSMode(t *testing.T) {
	dir := t.TempDir()
	// Simulates a config.json written before lan.dhcp.dns_mode existed.
	raw := `{
		"system":{"hostname":"router","admin":{"password_hash":"$2a$10$abc","must_change":false}},
		"wan":{"mode":"dhcp","interface":"eth0","static":{"dns":[]},"pppoe":{},"modem_ip":"auto"},
		"lan":{"interfaces":["eth1"],"address":"192.168.1.1/24","dhcp":{"enabled":true,"start":"192.168.1.100","end":"192.168.1.200","lease":"12h"}},
		"ipv6":{"enabled":false,"lan_prefix_len":"auto"},
		"dns":{"upstreams":[]}
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o640); err != nil { //nolint:gosec // G306: matching production 0640 permission in test fixture
		t.Fatal(err)
	}
	s, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if got := s.Get().LAN.DHCP.DNSMode; got != "auto" {
		t.Errorf("dns_mode after load = %q; want auto (backward-compat default)", got)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "valid dhcp",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "static missing address",
			cfg: config.Config{
				WAN: config.WAN{Mode: "static"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "pppoe missing username",
			cfg: config.Config{
				WAN: config.WAN{Mode: "pppoe"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "invalid mode",
			cfg: config.Config{
				WAN: config.WAN{Mode: "bogus"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "valid dns upstreams all schemes",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
				DNS: config.DNS{Upstreams: []string{
					"1.1.1.1",
					"1.1.1.1:5353",
					"tcp://8.8.8.8:53",
					"tls://dns.google:853",
					"https://dns.alidns.com/dns-query",
					"quic://dns.adguard.com:853",
					"h3://cloudflare-dns.com/dns-query",
				}},
			},
		},
		{
			name: "invalid dns upstream scheme",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
				DNS: config.DNS{Upstreams: []string{"ftp://1.1.1.1"}},
			},
			wantErr: true,
		},
		{
			name: "invalid dns upstream bare hostname",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
				DNS: config.DNS{Upstreams: []string{"dns.google"}},
			},
			wantErr: true,
		},
		{
			name: "dns_mode empty treated as auto",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "dns_mode auto explicit",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24", DHCP: config.DHCP{DNSMode: "auto"}},
			},
		},
		{
			name: "dns_mode manual with servers",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24", DHCP: config.DHCP{
					DNSMode:    "manual",
					DNSServers: []string{"1.1.1.1", "8.8.8.8"},
				}},
			},
		},
		{
			name: "dns_mode manual missing servers",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24", DHCP: config.DHCP{DNSMode: "manual"}},
			},
			wantErr: true,
		},
		{
			name: "dns_mode manual invalid server IP",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24", DHCP: config.DHCP{
					DNSMode:    "manual",
					DNSServers: []string{"not-an-ip"},
				}},
			},
			wantErr: true,
		},
		{
			name: "dns_mode invalid value",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24", DHCP: config.DHCP{DNSMode: "bogus"}},
			},
			wantErr: true,
		},
		{
			name: "wan mtu 0 is auto",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "wan mtu in range",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MTU: 1400},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "wan mtu below minimum",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MTU: 500},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "wan mtu above maximum",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MTU: 9201},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "wan mss 0 is auto",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp"},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "wan mss in range against default mtu 1500",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MSS: 1460},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "wan mss below minimum",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MSS: 500},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "wan mss exceeds mtu-40 default mtu",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MSS: 1461},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "wan mss exceeds mtu-40 pppoe default mtu",
			cfg: config.Config{
				WAN: config.WAN{Mode: "pppoe", MSS: 1453, PPPoE: config.PPPoEWAN{Username: "u"}},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "wan mss ok against pppoe default mtu",
			cfg: config.Config{
				WAN: config.WAN{Mode: "pppoe", MSS: 1452, PPPoE: config.PPPoEWAN{Username: "u"}},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
		{
			name: "wan mss exceeds explicit mtu override minus 40",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MTU: 1400, MSS: 1361},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
			wantErr: true,
		},
		{
			name: "wan mss ok against explicit mtu override minus 40",
			cfg: config.Config{
				WAN: config.WAN{Mode: "dhcp", MTU: 1400, MSS: 1360},
				LAN: config.LAN{Address: "192.168.1.1/24"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := config.Validate(&tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v; wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestParseDNSUpstream(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "1.1.1.1", want: "server 1.1.1.1"},
		{in: "1.1.1.1:5353", want: "server 1.1.1.1:5353"},
		{in: "2001:4860:4860::8888", want: "server 2001:4860:4860::8888"},
		{in: "[2001:4860:4860::8888]:53", want: "server [2001:4860:4860::8888]:53"},
		{in: "tcp://8.8.8.8", want: "server-tcp 8.8.8.8"},
		{in: "tcp://8.8.8.8:53", want: "server-tcp 8.8.8.8:53"},
		{in: "tls://223.5.5.5", want: "server-tls 223.5.5.5"},
		{in: "tls://dns.google:853", want: "server-tls dns.google:853"},
		{in: "https://dns.alidns.com/dns-query", want: "server-https https://dns.alidns.com/dns-query"},
		{in: "quic://dns.adguard.com:853", want: "server-quic dns.adguard.com:853"},
		{in: "h3://cloudflare-dns.com/dns-query", want: "server-h3 h3://cloudflare-dns.com/dns-query"},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "not-an-ip", wantErr: true},
		{in: "tcp://", wantErr: true},
		{in: "tls://host:notaport", wantErr: true},
		{in: "https://", wantErr: true},
		{in: "ftp://1.1.1.1", wantErr: true},
		{in: "1.1.1.1:99999", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := config.ParseDNSUpstream(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseDNSUpstream(%q) err = %v; wantErr %v", tc.in, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("ParseDNSUpstream(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNewStore_ConfiguredBackwardCompat verifies that loading a config.json
// written before the "configured" field existed (configured absent, defaults
// to false) but with a non-empty wan.interface is normalized to configured=true
// and the change is persisted so subsequent loads don't re-normalize.
func TestValidate_StaticDNSRecords(t *testing.T) {
	base := func(records []config.StaticDNSRecord) config.Config {
		return config.Config{
			WAN: config.WAN{Mode: "dhcp"},
			LAN: config.LAN{Address: "192.168.1.1/24"},
			DNS: config.DNS{StaticRecords: records},
		}
	}

	cases := []struct {
		name    string
		records []config.StaticDNSRecord
		wantErr bool
	}{
		{
			name:    "empty list is valid",
			records: nil,
		},
		{
			name:    "valid IPv4 single-label",
			records: []config.StaticDNSRecord{{Name: "nas.lan", IP: "192.168.20.10"}},
		},
		{
			name:    "valid IPv6",
			records: []config.StaticDNSRecord{{Name: "nas.lan", IP: "2001:db8::1"}},
		},
		{
			name:    "valid multi-label",
			records: []config.StaticDNSRecord{{Name: "a.b.lan", IP: "192.168.20.11"}},
		},
		{
			name: "two distinct records",
			records: []config.StaticDNSRecord{
				{Name: "nas.lan", IP: "192.168.20.10"},
				{Name: "cam.lan", IP: "192.168.20.11"},
			},
		},
		{
			name: "same name different IP is allowed",
			records: []config.StaticDNSRecord{
				{Name: "nas.lan", IP: "192.168.20.10"},
				{Name: "nas.lan", IP: "192.168.20.11"},
			},
		},
		{
			name: "duplicate same name+ip rejected",
			records: []config.StaticDNSRecord{
				{Name: "nas.lan", IP: "192.168.20.10"},
				{Name: "nas.lan", IP: "192.168.20.10"},
			},
			wantErr: true,
		},
		{
			name:    "reject .local — mDNS reservation",
			records: []config.StaticDNSRecord{{Name: "nas.local", IP: "192.168.20.10"}},
			wantErr: true,
		},
		{
			name:    "reject other TLD (.home)",
			records: []config.StaticDNSRecord{{Name: "nas.home", IP: "192.168.20.10"}},
			wantErr: true,
		},
		{
			name:    "reject reserved router.lan",
			records: []config.StaticDNSRecord{{Name: "router.lan", IP: "192.168.20.1"}},
			wantErr: true,
		},
		{
			name:    "reject reserved br0.lan",
			records: []config.StaticDNSRecord{{Name: "br0.lan", IP: "192.168.20.1"}},
			wantErr: true,
		},
		{
			name:    "reject reserved modem.lan",
			records: []config.StaticDNSRecord{{Name: "modem.lan", IP: "192.168.1.1"}},
			wantErr: true,
		},
		{
			name:    "reject invalid IP",
			records: []config.StaticDNSRecord{{Name: "nas.lan", IP: "not-an-ip"}},
			wantErr: true,
		},
		{
			name:    "reject invalid label (starts with dash)",
			records: []config.StaticDNSRecord{{Name: "-nas.lan", IP: "192.168.20.10"}},
			wantErr: true,
		},
		{
			name:    "reject bare label without .lan",
			records: []config.StaticDNSRecord{{Name: "nas", IP: "192.168.20.10"}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base(tc.records)
			err := config.Validate(&c)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err = %v; wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewStore_ConfiguredBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	// Simulate a pre-wizard config: no "configured" field, but wan.interface set.
	raw := `{
		"system":{"hostname":"router","admin":{"password_hash":"$2a$10$abc","must_change":false}},
		"wan":{"mode":"dhcp","interface":"eth0","static":{"dns":[]},"pppoe":{},"modem_ip":"auto"},
		"lan":{"interfaces":["eth1"],"address":"192.168.1.1/24","dhcp":{"enabled":true,"start":"192.168.1.100","end":"192.168.1.200","lease":"12h"}},
		"ipv6":{"enabled":false,"lan_prefix_len":"auto"},
		"dns":{"upstreams":[]}
	}`
	if err := os.WriteFile(dir+"/config.json", []byte(raw), 0o640); err != nil { //nolint:gosec // G306: test fixture
		t.Fatal(err)
	}

	s, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if !s.Get().System.Configured {
		t.Error("Configured should be true after normalization (wan.interface is set)")
	}

	// Reload to verify the normalization was persisted.
	s2, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !s2.Get().System.Configured {
		t.Error("Configured should remain true after reload (persisted by normalize)")
	}
}

// TestNewStore_ConfiguredFalseOnFreshSeed verifies that a freshly-seeded config
// (no config.json on disk) has configured=false.
func TestNewStore_ConfiguredFalseOnFreshSeed(t *testing.T) {
	dir := t.TempDir()
	s, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if s.Get().System.Configured {
		t.Error("fresh seed should have Configured=false")
	}
}

func TestSet_PersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	s, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Get()
	cfg.System.Hostname = "changed"
	if err := s.Set(cfg); err != nil {
		t.Fatalf("Set: %v", err)
	}

	s2, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := s2.Get().System.Hostname; got != "changed" {
		t.Errorf("reloaded hostname = %q; want changed", got)
	}
}
