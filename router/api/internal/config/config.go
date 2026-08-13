// Package config handles loading, saving, and validating the single source of truth: config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config is the top-level configuration schema. Field names must stay exactly as-is;
// the frontend depends on the JSON key spelling.
type Config struct {
	System System `json:"system"`
	WAN    WAN    `json:"wan"`
	LAN    LAN    `json:"lan"`
	IPv6   IPv6   `json:"ipv6"`
	DNS    DNS    `json:"dns"`
}

// System holds system-level settings such as hostname and admin credentials.
type System struct {
	Hostname string `json:"hostname"`
	Admin    Admin  `json:"admin"`
	// Configured is false on a freshly-installed system; the setup wizard sets
	// it to true when the user finishes the first-boot configuration flow.
	// Backward compat: if absent in an existing config.json but wan.interface is
	// set, normalize() treats the system as already configured and persists the
	// field so future loads are explicit.
	Configured bool `json:"configured"`
}

// Admin holds the admin account credentials.
type Admin struct {
	PasswordHash string `json:"password_hash"`
	MustChange   bool   `json:"must_change"`
}

// WAN holds wide-area network connection settings.
type WAN struct {
	Mode      string    `json:"mode"` // "dhcp" | "static" | "pppoe" | "bridge"
	Interface string    `json:"interface"`
	Static    StaticWAN `json:"static"`
	PPPoE     PPPoEWAN  `json:"pppoe"`
	ModemIP   string    `json:"modem_ip"` // "auto" or dotted-decimal
	// MTU overrides the WAN link MTU. 0 = auto (1492 for PPPoE, else 1500).
	MTU int `json:"mtu"`
	// MSS overrides the nftables TCP MSS clamp value. 0 = auto (clamp to
	// "rt mtu" i.e. the path MTU of the route); else the exact MSS to set.
	MSS int `json:"mss"`
}

// StaticWAN holds configuration for a static WAN IP address.
type StaticWAN struct {
	Address string   `json:"address"`
	Gateway string   `json:"gateway"`
	DNS     []string `json:"dns"`
}

// PPPoEWAN holds PPPoE dial-up credentials.
type PPPoEWAN struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LAN holds local area network settings.
type LAN struct {
	Interfaces []string `json:"interfaces"`
	Address    string   `json:"address"` // CIDR, e.g. "192.168.20.1/24"
	DHCP       DHCP     `json:"dhcp"`
}

// DHCP holds DHCP server configuration for the LAN.
type DHCP struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Lease   string `json:"lease"` // e.g. "12h"
	// DNSMode is "auto" (push the router's own LAN IP, i.e. SmartDNS) or
	// "manual" (push DNSServers instead). Empty is treated as "auto" for
	// backward compat with config.json files written before this field
	// existed; see Store.normalize.
	DNSMode string `json:"dns_mode"`
	// DNSServers is the DNS server list pushed to DHCP clients when
	// DNSMode == "manual". Ignored when DNSMode == "auto".
	DNSServers []string `json:"dns_servers"`
}

// IPv6 holds IPv6 and prefix-delegation settings.
type IPv6 struct {
	Enabled      bool        `json:"enabled"`
	LanPrefixLen interface{} `json:"lan_prefix_len"` // "auto" or number
}

// DNS holds upstream resolver configuration.
type DNS struct {
	Upstreams []string `json:"upstreams"`
}

// Store wraps the config with a mutex for concurrent access from HTTP handlers.
type Store struct {
	mu      sync.RWMutex
	cfg     Config
	cfgPath string
}

// NewStore loads the config from disk. If the file does not exist, it writes a default config.
func NewStore(baseDir string) (*Store, error) {
	path := filepath.Join(baseDir, "config.json")
	s := &Store{cfgPath: path}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from a trusted baseDir flag, not user input
	if errors.Is(err, os.ErrNotExist) {
		s.cfg = defaultConfig()
		if writeErr := s.save(); writeErr != nil {
			return nil, fmt.Errorf("write default config: %w", writeErr)
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &s.cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	changed := normalize(&s.cfg)
	if err := Validate(&s.cfg); err != nil {
		return nil, fmt.Errorf("invalid config on disk: %w", err)
	}
	if changed {
		// Persist any backward-compat defaults so future loads are explicit
		// (e.g. the configured field on pre-wizard config files).
		if writeErr := s.save(); writeErr != nil {
			return nil, fmt.Errorf("persist normalized config: %w", writeErr)
		}
	}
	return s, nil
}

// normalize fills in defaults for fields added after config.json files were
// already written on disk, so an old file doesn't fail Validate just because
// a newer field is missing/empty. Called on load, before Validate.
// Returns true if any field was changed (caller should persist the result).
func normalize(c *Config) (changed bool) {
	if c.LAN.DHCP.DNSMode == "" {
		c.LAN.DHCP.DNSMode = "auto"
		changed = true
	}
	// configured backward compat: files written before the wizard feature existed
	// lack the "configured" field (unmarshal leaves it false). If wan.interface is
	// non-empty the system was already configured; treat as true and persist so
	// future loads are explicit. A truly unconfigured system always has an empty
	// wan.interface in the default config seeded by NewStore.
	if !c.System.Configured && c.WAN.Interface != "" {
		c.System.Configured = true
		changed = true
	}
	return changed
}

// Get returns a deep copy of the current config.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Set validates and persists the given config.
func (s *Store) Set(cfg Config) error {
	if err := Validate(&cfg); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	return s.save()
}

// SetField updates the config via a mutator function without a full round-trip through JSON.
// The mutator must not hold the lock.
func (s *Store) SetField(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	if err := Validate(&s.cfg); err != nil {
		return err
	}
	return s.save()
}

// Path returns the path of config.json.
func (s *Store) Path() string { return s.cfgPath }

// save writes s.cfg to disk. Caller must hold s.mu (at least write-locked).
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically via temp file + rename so a partial write never corrupts config.json.
	tmp := s.cfgPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil { //nolint:gosec // G306: 0640 (owner rw, group r) is intentional so the web service group can read config.json
		return err
	}
	return os.Rename(tmp, s.cfgPath)
}

// Validate returns an error if the config is structurally invalid.
func Validate(c *Config) error {
	switch c.WAN.Mode {
	case "dhcp", "static", "pppoe", "bridge":
	default:
		return fmt.Errorf("wan.mode must be dhcp, static, pppoe, or bridge; got %q", c.WAN.Mode)
	}
	// wan.static/wan.pppoe are ignored in bridge mode (upstream router owns
	// DHCP/NAT); no address/credential validation applies.
	if c.WAN.Mode == "static" && c.WAN.Static.Address == "" {
		return errors.New("wan.static.address required when mode=static")
	}
	if c.WAN.Mode == "pppoe" && c.WAN.PPPoE.Username == "" {
		return errors.New("wan.pppoe.username required when mode=pppoe")
	}
	if c.LAN.Address == "" {
		return errors.New("lan.address is required")
	}
	switch c.LAN.DHCP.DNSMode {
	case "", "auto":
		// "" treated as "auto" for backward compat (see normalize); Validate
		// itself stays lenient so it can also be called directly (e.g. tests)
		// without going through Store's load-time normalization.
	case "manual":
		if len(c.LAN.DHCP.DNSServers) == 0 {
			return errors.New("lan.dhcp.dns_servers required when dns_mode=manual")
		}
		for i, ip := range c.LAN.DHCP.DNSServers {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("lan.dhcp.dns_servers[%d]: invalid IP %q", i, ip)
			}
		}
	default:
		return fmt.Errorf("lan.dhcp.dns_mode must be auto or manual; got %q", c.LAN.DHCP.DNSMode)
	}
	if c.WAN.MTU != 0 && (c.WAN.MTU < 576 || c.WAN.MTU > 9200) {
		return fmt.Errorf("wan.mtu must be 0 (auto) or between 576 and 9200; got %d", c.WAN.MTU)
	}
	if c.WAN.MSS != 0 {
		effectiveMTU := c.WAN.MTU
		if effectiveMTU == 0 {
			effectiveMTU = 1500
			if c.WAN.Mode == "pppoe" {
				effectiveMTU = 1492
			}
		}
		maxMSS := effectiveMTU - 40
		if c.WAN.MSS < 536 || c.WAN.MSS > maxMSS {
			return fmt.Errorf("wan.mss must be 0 (auto) or between 536 and %d (effective mtu %d minus 40); got %d", maxMSS, effectiveMTU, c.WAN.MSS)
		}
	}
	for i, u := range c.DNS.Upstreams {
		if _, err := ParseDNSUpstream(u); err != nil {
			return fmt.Errorf("dns.upstreams[%d]: %w", i, err)
		}
	}
	return nil
}

// dnsUpstreamFormats documents the accepted dns.upstreams entry syntaxes;
// included in validation errors so a rejected entry tells the user exactly
// what is expected.
const dnsUpstreamFormats = `supported formats: "IP" or "IP:port" (UDP), "tcp://host[:port]", "tls://host[:port]" (DoT), "https://host/path" (DoH), "quic://host[:port]", "h3://host/path"`

// schemeDirectives maps a upstream URL scheme to its smartdns directive for
// the host[:port]-only forms (tcp/tls/quic). https/h3 are handled separately
// since smartdns wants the full URL for those.
var schemeDirectives = map[string]string{
	"tcp":  "server-tcp",
	"tls":  "server-tls",
	"quic": "server-quic",
}

// ParseDNSUpstream validates a single dns.upstreams entry and returns the
// smartdns directive line it renders to, e.g. "server-tls dns.google:853" or
// "server-https https://dns.alidns.com/dns-query". It is the single source of
// truth for upstream syntax, used both by Validate (reject bad config on
// save) and by the apply package (compute the rendered directive line).
func ParseDNSUpstream(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("dns upstream entry is empty; %s", dnsUpstreamFormats)
	}

	scheme, rest, hasScheme := strings.Cut(s, "://")
	if !hasScheme {
		// Bare "server" entry: IP or IP:port. Hostnames are not accepted here
		// since smartdns's plain "server" directive expects an address.
		if err := validateHostPort(s, false); err != nil {
			return "", fmt.Errorf("invalid dns upstream %q: %s; %s", raw, err, dnsUpstreamFormats)
		}
		return "server " + s, nil
	}

	switch scheme {
	case "tcp", "tls", "quic":
		if err := validateHostPort(rest, true); err != nil {
			return "", fmt.Errorf("invalid dns upstream %q: %s; %s", raw, err, dnsUpstreamFormats)
		}
		return schemeDirectives[scheme] + " " + rest, nil
	case "https", "h3":
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid dns upstream %q: malformed URL; %s", raw, dnsUpstreamFormats)
		}
		directive := "server-https"
		if scheme == "h3" {
			directive = "server-h3"
		}
		return directive + " " + s, nil
	default:
		return "", fmt.Errorf("invalid dns upstream %q: unsupported scheme %q; %s", raw, scheme, dnsUpstreamFormats)
	}
}

// validateHostPort checks a "host" or "host:port" string (IPv6 hosts use the
// bracketed "[host]:port" form). allowHostname permits DNS names in addition
// to IP literals; the bare "server" directive (no scheme) requires an IP.
func validateHostPort(s string, allowHostname bool) error {
	host := s
	port := ""
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, port = h, p
	}
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid port %q", port)
		}
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	if !allowHostname {
		return fmt.Errorf("host %q is not a valid IP address", host)
	}
	if !isValidHostname(host) {
		return fmt.Errorf("host %q is not a valid IP address or hostname", host)
	}
	return nil
}

// isValidHostname reports whether h is a syntactically valid DNS hostname
// (RFC 1123 labels joined by dots).
func isValidHostname(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 {
			return false
		}
		for i, c := range l {
			switch {
			case c == '-':
				if i == 0 || i == len(l)-1 {
					return false
				}
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
				// ok
			default:
				return false
			}
		}
	}
	return true
}

func defaultConfig() Config {
	return Config{
		System: System{
			Hostname: "router",
			Admin: Admin{
				// bcrypt hash of "password" — generated at startup if empty; placeholder only.
				PasswordHash: "",
				MustChange:   true,
			},
		},
		WAN: WAN{
			Mode:    "dhcp",
			ModemIP: "auto",
		},
		LAN: LAN{
			Address: "192.168.20.1/24",
			DHCP: DHCP{
				Enabled: true,
				Start:   "192.168.20.100",
				End:     "192.168.20.254",
				Lease:   "12h",
				DNSMode: "auto",
			},
		},
		IPv6: IPv6{
			Enabled:      false,
			LanPrefixLen: "auto",
		},
		DNS: DNS{
			Upstreams: []string{"119.29.29.29", "223.5.5.5"},
		},
	}
}
