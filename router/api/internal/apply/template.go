// Package apply handles template rendering, dry-run validation, and the apply pipeline.
package apply

import (
	"fmt"
	"luban/internal/config"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// templateFuncs is registered on every parsed template. "join" mirrors
// strings.Join, needed to render comma-separated lists (e.g. DhcpDNSServers)
// from text/template, which has no built-in join.
var templateFuncs = template.FuncMap{
	"join": strings.Join,
}

// TemplateData is the render context passed into every service config template.
// Computed fields reduce template complexity — logic stays in Go, not Go template syntax.
type TemplateData struct {
	config.Config

	// WanIface is the WAN interface name (e.g. "eth0" or "ppp0" for PPPoE).
	WanIface string
	// LanIface is "br0" when multiple LAN ports exist, otherwise the lone LAN port name.
	LanIface string
	// LanIP is the LAN address without the prefix length (e.g. "192.168.20.1").
	LanIP string
	// LanCIDR is the full CIDR (e.g. "192.168.20.1/24").
	LanCIDR string
	// LanPrefixLen is the LAN address prefix length (e.g. 24 for a /24).
	LanPrefixLen int
	// WanAddress is the static WAN address without the prefix length; empty unless mode=static.
	WanAddress string
	// WanPrefixLen is the static WAN address prefix length; zero unless mode=static.
	WanPrefixLen int
	// Ipv6PrefixLen is the requested delegated-prefix length; zero when unset or "auto".
	Ipv6PrefixLen int
	// MTU is 1492 for PPPoE, 1500 otherwise.
	MTU int
	// ModemIP is the resolved modem IP; empty string if auto-detection not done yet.
	ModemIP string
	// Port is the current LAN interface name; only set while rendering a per-port template.
	Port string
	// DNSUpstreamLines is DNS.Upstreams converted to full smartdns directive
	// lines, e.g. "server-tls dns.google:853" (see config.ParseDNSUpstream).
	DNSUpstreamLines []string
	// DhcpDNSServers is the DNS server list pushed to DHCP clients via
	// option 6: [LanIP] (self, i.e. SmartDNS) when lan.dhcp.dns_mode is
	// "auto", or lan.dhcp.dns_servers verbatim when "manual".
	DhcpDNSServers []string
	// MSSClampLine is the trailing operand of the nftables TCP MSS clamp
	// statement ("tcp flags syn tcp option maxseg size set <MSSClampLine>"):
	// "rt mtu" when wan.mss is 0 (auto, clamp to path MTU), or the decimal
	// wan.mss value otherwise.
	MSSClampLine string

	// Boolean helpers used in templates.
	IsPPPoE  bool
	IsStatic bool
	IsDHCP   bool
	// IsBridge is true when wan.mode == "bridge" (wired-bridge): the
	// WAN port joins br0 and the router degrades to a switch/AP — the
	// upstream router owns DHCP/NAT, this device only takes a management
	// address on br0 via DHCP client.
	IsBridge bool
	// IsUnconfigured is true during first-boot wizard mode: no WAN port is
	// assigned yet, all detected ethernet ports are enslaved to br0, and the
	// nftables/sysctl templates omit WAN-specific rules. Set by
	// BuildUnconfiguredTemplateData; never set by BuildTemplateData.
	IsUnconfigured bool
}

// BuildTemplateData constructs the render context from the stored config.
func BuildTemplateData(cfg config.Config) (TemplateData, error) {
	d := TemplateData{Config: cfg}

	d.IsDHCP = cfg.WAN.Mode == "dhcp"
	d.IsStatic = cfg.WAN.Mode == "static"
	d.IsPPPoE = cfg.WAN.Mode == "pppoe"
	d.IsBridge = cfg.WAN.Mode == "bridge"

	d.WanIface = cfg.WAN.Interface
	if d.WanIface == "" {
		d.WanIface = "eth0"
	}

	switch len(cfg.LAN.Interfaces) {
	case 0:
		return d, fmt.Errorf("lan.interfaces must have at least one entry")
	case 1:
		d.LanIface = cfg.LAN.Interfaces[0]
	default:
		d.LanIface = "br0"
	}

	d.LanCIDR = cfg.LAN.Address
	lanIP, lanNet, err := net.ParseCIDR(cfg.LAN.Address)
	if err != nil {
		return d, fmt.Errorf("invalid lan.address %q: %w", cfg.LAN.Address, err)
	}
	d.LanIP = lanIP.String()
	d.LanPrefixLen, _ = lanNet.Mask.Size()

	if cfg.LAN.DHCP.DNSMode == "manual" {
		d.DhcpDNSServers = cfg.LAN.DHCP.DNSServers
	} else {
		d.DhcpDNSServers = []string{d.LanIP}
	}

	if d.IsStatic {
		wanIP, wanNet, err := net.ParseCIDR(cfg.WAN.Static.Address)
		if err != nil {
			return d, fmt.Errorf("invalid wan.static.address %q: %w", cfg.WAN.Static.Address, err)
		}
		d.WanAddress = wanIP.String()
		d.WanPrefixLen, _ = wanNet.Mask.Size()
	}

	d.Ipv6PrefixLen = parseIpv6PrefixLen(cfg.IPv6.LanPrefixLen)

	switch {
	case cfg.WAN.MTU > 0:
		d.MTU = cfg.WAN.MTU
	case d.IsPPPoE:
		d.MTU = 1492
	default:
		d.MTU = 1500
	}

	if cfg.WAN.MSS > 0 {
		d.MSSClampLine = strconv.Itoa(cfg.WAN.MSS)
	} else {
		d.MSSClampLine = "rt mtu"
	}

	if cfg.WAN.ModemIP == "auto" || cfg.WAN.ModemIP == "" {
		d.ModemIP = ""
	} else {
		d.ModemIP = cfg.WAN.ModemIP
	}

	d.DNSUpstreamLines = make([]string, 0, len(cfg.DNS.Upstreams))
	for _, u := range cfg.DNS.Upstreams {
		line, err := config.ParseDNSUpstream(u)
		if err != nil {
			// Should already be rejected by config.Validate at save time;
			// re-checked here so a bad on-disk config.json fails loudly
			// instead of rendering a malformed smartdns.conf.
			return d, fmt.Errorf("dns.upstreams: %w", err)
		}
		d.DNSUpstreamLines = append(d.DNSUpstreamLines, line)
	}

	return d, nil
}

// parseIpv6PrefixLen normalizes the "auto" | number union type into an int.
// Returns 0 (falsy in templates) for "auto", empty, or unparseable values.
func parseIpv6PrefixLen(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// RenderEntry maps a template file name to its output path on the deployed system.
type RenderEntry struct {
	// TemplateFile is the filename inside <basedir>/templates/.
	TemplateFile string
	// OutputPath is the absolute path on the filesystem where the rendered file is written.
	OutputPath string
	// Mode is the file permission bits. 0 means use 0644.
	Mode os.FileMode
	// Port is non-empty for a per-LAN-port entry; TemplateData.Port is set to it before render.
	Port string
}

// lanPortTemplateFile is rendered once per LAN interface (see expandRenderTable).
const lanPortTemplateFile = "networkd-lan-port.network.tpl"

// renderTable is the canonical mapping of template → output path.
// Template authors must keep filenames consistent with these entries.
// It does NOT include the per-LAN-port entries; those are appended by
// expandRenderTable once the set of LAN interfaces is known.
var renderTable = []RenderEntry{
	{TemplateFile: "networkd-wan.network.tpl", OutputPath: "/etc/systemd/network/10-wan.network"},
	{TemplateFile: "networkd-br0.netdev.tpl", OutputPath: "/etc/systemd/network/15-br0.netdev"},
	{TemplateFile: "networkd-br0.network.tpl", OutputPath: "/etc/systemd/network/20-br0.network"},
	{TemplateFile: "networkd-ppp0.network.tpl", OutputPath: "/etc/systemd/network/30-ppp0.network"},
	{TemplateFile: "dnsmasq.conf.tpl", OutputPath: "/etc/dnsmasq.d/router.conf"},
	{TemplateFile: "smartdns.conf.tpl", OutputPath: "/etc/smartdns/smartdns.conf"},
	{TemplateFile: "nftables.conf.tpl", OutputPath: "/etc/nftables.conf"},
	{TemplateFile: "sysctl.conf.tpl", OutputPath: "/etc/sysctl.d/99-router.conf"},
	{TemplateFile: "ppp-peers-wan.tpl", OutputPath: "/etc/ppp/peers/wan"},
	{TemplateFile: "chap-secrets.tpl", OutputPath: "/etc/ppp/chap-secrets", Mode: 0o600},
	{TemplateFile: "Caddyfile.tpl", OutputPath: "/etc/caddy/Caddyfile"},
}

// expandRenderTable returns renderTable plus one entry per LAN interface for the
// per-port bridge-enslavement template. Numeric prefixes (40, 41, …) keep
// networkd's lexical file ordering sensible. router-rollback.sh restores these
// (and every other .network/.netdev file) via a generic
// "/etc/systemd/network/*.network.bak" / "*.netdev.bak" glob, matching the
// ".bak"-beside-each-file backup scheme used by installRendered/Rollback in
// pipeline.go.
func expandRenderTable(data TemplateData) []RenderEntry {
	entries := make([]RenderEntry, 0, len(renderTable)+len(data.LAN.Interfaces))
	entries = append(entries, renderTable...)
	for i, port := range data.LAN.Interfaces {
		entries = append(entries, RenderEntry{
			TemplateFile: lanPortTemplateFile,
			OutputPath:   fmt.Sprintf("/etc/systemd/network/4%d-lan-%s.network", i, port),
			Port:         port,
		})
	}
	return entries
}

// RenderAll renders all templates into a temporary directory.
// Returns the temp dir path and the concrete entries that were rendered
// (including per-LAN-port expansion); caller is responsible for tmpDir cleanup.
// Each missing template file results in a clear error, not a panic.
func RenderAll(baseDir string, data TemplateData) (tmpDir string, entries []RenderEntry, err error) {
	tmpDir, err = os.MkdirTemp("", "luban-render-*")
	if err != nil {
		return "", nil, fmt.Errorf("mktemp: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			tmpDir = ""
		}
	}()

	entries = expandRenderTable(data)

	// Cache parsed templates by filename since the per-port template is reused
	// across multiple entries.
	parsed := make(map[string]*template.Template, len(renderTable)+1)

	for _, e := range entries {
		t, ok := parsed[e.TemplateFile]
		if !ok {
			tplPath := filepath.Join(baseDir, "templates", e.TemplateFile)
			tplSrc, readErr := os.ReadFile(tplPath) //nolint:gosec // G304: tplPath is composed from trusted baseDir + hardcoded template filenames from the render table
			if readErr != nil {
				if os.IsNotExist(readErr) {
					return "", nil, fmt.Errorf("template %q not found at %s", e.TemplateFile, tplPath)
				}
				return "", nil, fmt.Errorf("read template %q: %w", e.TemplateFile, readErr)
			}
			t, err = template.New(e.TemplateFile).Funcs(templateFuncs).Parse(string(tplSrc))
			if err != nil {
				return "", nil, fmt.Errorf("parse template %q: %w", e.TemplateFile, err)
			}
			parsed[e.TemplateFile] = t
		}

		// Mirror the output path hierarchy inside tmpDir.
		outRel := strings.TrimPrefix(e.OutputPath, "/")
		outAbs := filepath.Join(tmpDir, outRel)
		if mkdirErr := os.MkdirAll(filepath.Dir(outAbs), 0o755); mkdirErr != nil { //nolint:gosec // G301: temp dir hierarchy mirrors system paths; world-readable is appropriate
			return "", nil, fmt.Errorf("mkdir for %s: %w", e.TemplateFile, mkdirErr)
		}

		mode := e.Mode
		if mode == 0 {
			mode = 0o644
		}
		f, createErr := os.OpenFile(outAbs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // G304: outAbs is inside our own temp dir, not derived from user input
		if createErr != nil {
			return "", nil, fmt.Errorf("create output for %s: %w", e.TemplateFile, createErr)
		}

		renderData := data
		renderData.Port = e.Port
		execErr := t.Execute(f, renderData)
		_ = f.Close()
		if execErr != nil {
			err = fmt.Errorf("render template %q: %w", e.TemplateFile, execErr)
			return "", nil, err
		}
	}

	return tmpDir, entries, nil
}

// BuildUnconfiguredTemplateData constructs TemplateData for first-boot
// unconfigured mode. All detectedPorts are enslaved to br0 so the wizard
// is reachable from any ethernet port. The WAN interface is left empty;
// templates check IsUnconfigured to skip WAN-specific output.
//
// detectedPorts should be the ethernet-only interface names returned by
// detect.EnumerateInterfaces (Wifi=false). If the slice is empty, a single
// placeholder "eth0" is used so the render pipeline does not error on an
// empty LAN interface list.
func BuildUnconfiguredTemplateData(cfg config.Config, detectedPorts []string) (TemplateData, error) {
	if len(detectedPorts) == 0 {
		detectedPorts = []string{"eth0"}
	}

	const lanAddr = "192.168.20.1/24"
	lanIP, lanNet, err := net.ParseCIDR(lanAddr)
	if err != nil {
		return TemplateData{}, fmt.Errorf("parse default lan address: %w", err)
	}
	lanPrefixLen, _ := lanNet.Mask.Size()

	// Derive DNS upstream lines from the stored config (carry over upstreams if
	// already set; fall back to the hard-coded defaults for a truly blank config).
	upstreams := cfg.DNS.Upstreams
	if len(upstreams) == 0 {
		upstreams = []string{"119.29.29.29", "223.5.5.5"}
	}
	dnsLines := make([]string, 0, len(upstreams))
	for _, u := range upstreams {
		line, parseErr := config.ParseDNSUpstream(u)
		if parseErr != nil {
			continue
		}
		dnsLines = append(dnsLines, line)
	}

	ucfg := cfg
	ucfg.LAN.Interfaces = detectedPorts
	ucfg.LAN.Address = lanAddr
	ucfg.LAN.DHCP = config.DHCP{
		Enabled: true,
		Start:   "192.168.20.100",
		End:     "192.168.20.254",
		Lease:   "12h",
		DNSMode: "auto",
	}

	return TemplateData{
		Config:         ucfg,
		IsUnconfigured: true,
		// No WAN interface in unconfigured mode.
		WanIface:     "",
		LanIface:     "br0",
		LanIP:        lanIP.String(),
		LanCIDR:      lanAddr,
		LanPrefixLen: lanPrefixLen,
		// Default 1500; pppoe not in use, no mtu override.
		MTU:              1500,
		MSSClampLine:     "rt mtu",
		DhcpDNSServers:   []string{lanIP.String()},
		DNSUpstreamLines: dnsLines,
	}, nil
}

// RenderOne renders a single named template to a string (used in tests).
func RenderOne(tplSrc string, data TemplateData) (string, error) {
	t, err := template.New("test").Funcs(templateFuncs).Parse(tplSrc)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}
