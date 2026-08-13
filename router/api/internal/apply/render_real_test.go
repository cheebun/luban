package apply_test

// TestRenderAll_RealTemplates renders every template that actually ships in
// router/templates/ against the real renderTable/expandRenderTable logic, for
// all three WAN modes. This is the regression test that would have caught the
// contract mismatches between the Go structs, apply/template.go's renderTable,
// and the {{...}} field references inside router/templates/*.tpl: wrong
// on-disk filenames, wrong struct field casing/paths, and the missing
// per-LAN-port / br0.netdev entries.
//
// It is repo-layout dependent (walks up from this package to find
// router/templates/), so it skips cleanly if that directory isn't present
// (e.g. when internal/apply is vendored or copied out of the monorepo).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"luban/internal/apply"
	"luban/internal/config"
)

// readRendered reads a rendered output file (given its absolute deployed
// path, e.g. "/etc/dnsmasq.d/router.conf") back out of the RenderAll tmpDir.
func readRendered(t *testing.T, tmpDir, outputPath string) string {
	t.Helper()
	outRel := filepath.FromSlash(strings.TrimPrefix(outputPath, "/"))
	data, err := os.ReadFile(filepath.Join(tmpDir, outRel))
	if err != nil {
		t.Fatalf("read rendered %s: %v", outputPath, err)
	}
	return string(data)
}

// realTemplatesDir locates router/templates/ relative to this test file.
// This file lives at router/api/internal/apply/, so router/templates/ is
// three levels up plus "templates".
func realTemplatesDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "templates")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("router/templates/ not found at %s (repo layout unavailable): %v", dir, err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve abs path for %s: %v", dir, err)
	}
	return abs
}

// sampleConfig returns a fully populated config for the given WAN mode.
// Every field referenced by any template is set to a realistic, non-zero
// value so a bad {{ }} reference fails loudly instead of silently rendering
// empty.
func sampleConfig(mode string) config.Config {
	cfg := config.Config{
		System: config.System{
			Hostname: "router",
			Admin:    config.Admin{PasswordHash: "$2a$10$abc", MustChange: false},
		},
		WAN: config.WAN{
			Mode:      mode,
			Interface: "eth0",
			ModemIP:   "192.168.1.1",
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
		IPv6: config.IPv6{Enabled: true, LanPrefixLen: float64(60)},
		DNS: config.DNS{Upstreams: []string{
			"119.29.29.29",                      // plain UDP IP
			"tcp://8.8.8.8:53",                  // server-tcp
			"tls://dns.google:853",              // server-tls (DoT)
			"https://dns.alidns.com/dns-query",  // server-https (DoH)
			"quic://dns.adguard.com:853",        // server-quic
			"h3://cloudflare-dns.com/dns-query", // server-h3
		}},
	}

	switch mode {
	case "static":
		cfg.WAN.Static = config.StaticWAN{
			Address: "203.0.113.5/24",
			Gateway: "203.0.113.1",
			DNS:     []string{"8.8.8.8"},
		}
	case "pppoe":
		cfg.WAN.PPPoE = config.PPPoEWAN{
			Username: "user@isp",
			Password: "secret",
		}
	}

	return cfg
}

func TestRenderAll_RealTemplates(t *testing.T) {
	baseDir := filepath.Dir(realTemplatesDir(t)) // parent of templates/, i.e. RenderAll's baseDir

	for _, mode := range []string{"dhcp", "static", "pppoe", "bridge"} {
		t.Run(mode, func(t *testing.T) {
			cfg := sampleConfig(mode)
			data, err := apply.BuildTemplateData(cfg)
			if err != nil {
				t.Fatalf("BuildTemplateData(%s): %v", mode, err)
			}

			tmpDir, entries, err := apply.RenderAll(baseDir, data)
			if err != nil {
				t.Fatalf("RenderAll(%s): %v", mode, err)
			}
			defer os.RemoveAll(tmpDir)

			if len(entries) == 0 {
				t.Fatal("expected at least one rendered entry")
			}

			// Every non-mode-specific entry must produce a non-empty file; the
			// mode-specific ones (ppp0.network, ppp-peers-wan, chap-secrets) are
			// still rendered (RenderAll renders unconditionally — installRendered
			// is what skips them at apply time), so just confirm all files exist.
			for _, e := range entries {
				outRel := filepath.FromSlash(e.OutputPath[1:])
				outAbs := filepath.Join(tmpDir, outRel)
				info, statErr := os.Stat(outAbs)
				if statErr != nil {
					t.Errorf("mode=%s template %q: output %s not written: %v", mode, e.TemplateFile, e.OutputPath, statErr)
					continue
				}
				if info.Size() == 0 {
					t.Errorf("mode=%s template %q: output %s is empty", mode, e.TemplateFile, e.OutputPath)
				}
			}

			// Sanity-check the per-LAN-port expansion: one entry per LAN.Interfaces.
			portEntries := 0
			for _, e := range entries {
				if e.Port != "" {
					portEntries++
				}
			}
			if portEntries != len(cfg.LAN.Interfaces) {
				t.Errorf("mode=%s: got %d per-port entries, want %d", mode, portEntries, len(cfg.LAN.Interfaces))
			}
		})
	}
}

// TestRenderAll_RealTemplates_ManualDNS renders with lan.dhcp.dns_mode=manual
// and checks dnsmasq.conf.tpl pushes the configured servers instead of the
// router's own LAN IP.
func TestRenderAll_RealTemplates_ManualDNS(t *testing.T) {
	baseDir := filepath.Dir(realTemplatesDir(t))

	cfg := sampleConfig("dhcp")
	cfg.LAN.DHCP.DNSMode = "manual"
	cfg.LAN.DHCP.DNSServers = []string{"1.1.1.1", "8.8.8.8"}

	data, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	tmpDir, _, err := apply.RenderAll(baseDir, data)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dnsmasqConf := readRendered(t, tmpDir, "/etc/dnsmasq.d/router.conf")
	want := "dhcp-option=option:dns-server,1.1.1.1,8.8.8.8"
	if !strings.Contains(dnsmasqConf, want) {
		t.Errorf("dnsmasq.conf missing %q; got:\n%s", want, dnsmasqConf)
	}
	if strings.Contains(dnsmasqConf, "dhcp-option=option:dns-server,192.168.20.1") {
		t.Errorf("dnsmasq.conf still advertises router LAN IP as DNS in manual mode:\n%s", dnsmasqConf)
	}
}

// TestRenderAll_RealTemplates_MTUOverride renders with wan.mtu=1400 and
// checks the override propagates to the WAN link, br0 RA MTU, and DHCP
// option 26 (MTU), all of which key off the effective MTU rather than
// hardcoded PPPoE-only logic.
func TestRenderAll_RealTemplates_MTUOverride(t *testing.T) {
	baseDir := filepath.Dir(realTemplatesDir(t))

	cfg := sampleConfig("dhcp")
	cfg.WAN.MTU = 1400

	data, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	tmpDir, _, err := apply.RenderAll(baseDir, data)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	wanNet := readRendered(t, tmpDir, "/etc/systemd/network/10-wan.network")
	if !strings.Contains(wanNet, "MTUBytes=1400") {
		t.Errorf("10-wan.network missing MTUBytes=1400; got:\n%s", wanNet)
	}

	br0Net := readRendered(t, tmpDir, "/etc/systemd/network/20-br0.network")
	if !strings.Contains(br0Net, "MTU=1400") {
		t.Errorf("20-br0.network missing MTU=1400; got:\n%s", br0Net)
	}

	dnsmasqConf := readRendered(t, tmpDir, "/etc/dnsmasq.d/router.conf")
	if !strings.Contains(dnsmasqConf, "dhcp-option=option:mtu,1400") {
		t.Errorf("dnsmasq.conf missing option:mtu,1400; got:\n%s", dnsmasqConf)
	}
}

// TestRenderAll_RealTemplates_MSSOverride renders with wan.mss set and checks
// the nftables MSS clamp uses the fixed value instead of "rt mtu".
func TestRenderAll_RealTemplates_MSSOverride(t *testing.T) {
	baseDir := filepath.Dir(realTemplatesDir(t))

	cfg := sampleConfig("dhcp")
	cfg.WAN.MSS = 1360

	data, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	tmpDir, _, err := apply.RenderAll(baseDir, data)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nft := readRendered(t, tmpDir, "/etc/nftables.conf")
	want := "tcp option maxseg size set 1360"
	if !strings.Contains(nft, want) {
		t.Errorf("nftables.conf missing %q; got:\n%s", want, nft)
	}
	if strings.Contains(nft, "size set rt mtu") {
		t.Errorf("nftables.conf still clamps to rt mtu with wan.mss set:\n%s", nft)
	}
}

// TestRenderAll_RealTemplates_Bridge renders wan.mode=bridge and checks the
// WAN port joins br0 as a plain bridge port, br0 becomes a DHCP client (no
// static address / PD / RA), dnsmasq's DHCP server is fully disabled, and
// nftables drops masquerade/MSS-clamp/modem-SNAT in favor of a minimal
// self-protection ruleset with forward ACCEPT.
func TestRenderAll_RealTemplates_Bridge(t *testing.T) {
	baseDir := filepath.Dir(realTemplatesDir(t))

	cfg := sampleConfig("bridge")

	data, err := apply.BuildTemplateData(cfg)
	if err != nil {
		t.Fatalf("BuildTemplateData: %v", err)
	}
	if !data.IsBridge {
		t.Fatal("IsBridge should be true for wan.mode=bridge")
	}

	tmpDir, _, err := apply.RenderAll(baseDir, data)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	wanNet := readRendered(t, tmpDir, "/etc/systemd/network/10-wan.network")
	if !strings.Contains(wanNet, "Bridge=br0") {
		t.Errorf("10-wan.network missing Bridge=br0 in bridge mode; got:\n%s", wanNet)
	}
	if strings.Contains(wanNet, "DHCP=yes") || strings.Contains(wanNet, "DHCP=ipv6") {
		t.Errorf("10-wan.network should not run a WAN-side DHCP/DHCPv6 client in bridge mode; got:\n%s", wanNet)
	}

	br0Net := readRendered(t, tmpDir, "/etc/systemd/network/20-br0.network")
	if !strings.Contains(br0Net, "DHCP=yes") {
		t.Errorf("20-br0.network missing DHCP=yes in bridge mode; got:\n%s", br0Net)
	}
	if strings.Contains(br0Net, "Address=") || strings.Contains(br0Net, "DHCPPrefixDelegation") || strings.Contains(br0Net, "IPv6SendRA=yes") {
		t.Errorf("20-br0.network should have no static address/PD/RA in bridge mode; got:\n%s", br0Net)
	}
	if !strings.Contains(br0Net, "ConfigureWithoutCarrier=yes") {
		t.Errorf("20-br0.network missing ConfigureWithoutCarrier=yes in bridge mode; got:\n%s", br0Net)
	}

	dnsmasqConf := readRendered(t, tmpDir, "/etc/dnsmasq.d/router.conf")
	if !strings.Contains(dnsmasqConf, "port=0") {
		t.Errorf("dnsmasq.conf missing port=0 in bridge mode; got:\n%s", dnsmasqConf)
	}
	if strings.Contains(dnsmasqConf, "dhcp-range=") {
		t.Errorf("dnsmasq.conf should not emit dhcp-range in bridge mode; got:\n%s", dnsmasqConf)
	}

	nft := readRendered(t, tmpDir, "/etc/nftables.conf")
	if strings.Contains(nft, "table ip nat") {
		t.Errorf("nftables.conf should not define a nat table (no masquerade) in bridge mode; got:\n%s", nft)
	}
	if strings.Contains(nft, "maxseg size set") {
		t.Errorf("nftables.conf should not MSS-clamp in bridge mode; got:\n%s", nft)
	}
	if !strings.Contains(nft, "policy accept;") {
		t.Errorf("nftables.conf forward chain should be policy accept in bridge mode; got:\n%s", nft)
	}

	sysctlConf := readRendered(t, tmpDir, "/etc/sysctl.d/99-router.conf")
	if strings.Contains(sysctlConf, "accept_redirects") || strings.Contains(sysctlConf, ".accept_ra = 2") {
		t.Errorf("99-router.conf should skip WAN-specific accept_ra/redirects tuning in bridge mode; got:\n%s", sysctlConf)
	}
	if !strings.Contains(sysctlConf, "net.ipv4.ip_forward = 1") {
		t.Errorf("99-router.conf should still keep ip_forward=1 (harmless) in bridge mode; got:\n%s", sysctlConf)
	}
}

// TestRenderAll_RealTemplates_BridgeRoundTrip renders bridge then routed
// (dhcp) then bridge again, checking each direction produces the mode's
// expected output — i.e. switching away from bridge and back doesn't leave
// stale content from the other mode (all these templates are re-rendered in
// full on every apply, not patched incrementally).
func TestRenderAll_RealTemplates_BridgeRoundTrip(t *testing.T) {
	baseDir := filepath.Dir(realTemplatesDir(t))

	render := func(mode string) (wanNet, br0Net string) {
		cfg := sampleConfig(mode)
		data, err := apply.BuildTemplateData(cfg)
		if err != nil {
			t.Fatalf("BuildTemplateData(%s): %v", mode, err)
		}
		tmpDir, _, err := apply.RenderAll(baseDir, data)
		if err != nil {
			t.Fatalf("RenderAll(%s): %v", mode, err)
		}
		defer os.RemoveAll(tmpDir)
		return readRendered(t, tmpDir, "/etc/systemd/network/10-wan.network"),
			readRendered(t, tmpDir, "/etc/systemd/network/20-br0.network")
	}

	bridgeWan, bridgeBr0 := render("bridge")
	if !strings.Contains(bridgeWan, "Bridge=br0") {
		t.Errorf("bridge->: 10-wan.network missing Bridge=br0:\n%s", bridgeWan)
	}
	if !strings.Contains(bridgeBr0, "DHCP=yes") {
		t.Errorf("bridge->: 20-br0.network missing DHCP=yes:\n%s", bridgeBr0)
	}

	routedWan, routedBr0 := render("dhcp")
	if strings.Contains(routedWan, "Bridge=br0") {
		t.Errorf("bridge->routed: 10-wan.network still has Bridge=br0 (stale):\n%s", routedWan)
	}
	if !strings.Contains(routedWan, "DHCP=yes") {
		t.Errorf("bridge->routed: 10-wan.network missing DHCP=yes:\n%s", routedWan)
	}
	if strings.Contains(routedBr0, "\nDHCP=yes") {
		t.Errorf("bridge->routed: 20-br0.network still has DHCP=yes (stale):\n%s", routedBr0)
	}
	if !strings.Contains(routedBr0, "DHCPPrefixDelegation=yes") {
		t.Errorf("bridge->routed: 20-br0.network missing DHCPPrefixDelegation=yes:\n%s", routedBr0)
	}

	backToBridgeWan, backToBridgeBr0 := render("bridge")
	if !strings.Contains(backToBridgeWan, "Bridge=br0") {
		t.Errorf("routed->bridge: 10-wan.network missing Bridge=br0:\n%s", backToBridgeWan)
	}
	if !strings.Contains(backToBridgeBr0, "DHCP=yes") || strings.Contains(backToBridgeBr0, "DHCPPrefixDelegation") {
		t.Errorf("routed->bridge: 20-br0.network not clean bridge output (stale PD?):\n%s", backToBridgeBr0)
	}
}
