// Package status collects live network/system status by running system commands.
// All commands degrade gracefully on macOS or when binaries are absent — callers
// receive nil/zero values rather than errors so the /api/status endpoint always responds.
package status

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const cmdTimeout = 5 * time.Second

// Status is the full status payload returned by GET /api/status.
type Status struct {
	Uptime   string          `json:"uptime"`
	Addrs    []AddrInfo      `json:"addrs"`
	Routes   []RouteInfo     `json:"routes"`
	Nftables json.RawMessage `json:"nftables,omitempty"` // raw output of nft -j list ruleset
	Leases   []DHCPLease     `json:"leases"`
	Services []ServiceInfo   `json:"services"`
	System   SystemInfo      `json:"system"`
}

// SystemInfo is the dashboard "system overview" block. Every optional field
// is a pointer so it degrades to JSON null (rather than a misleading zero
// value) when the underlying source is unavailable, e.g. running on macOS
// in dev where /proc and /sys don't exist. The frontend renders null as "unknown".
type SystemInfo struct {
	Hostname   string          `json:"hostname"`
	OS         *string         `json:"os"`     // PRETTY_NAME from /etc/os-release
	Kernel     *string         `json:"kernel"` // uname -r
	Arch       *string         `json:"arch"`   // uname -m
	Uptime     string          `json:"uptime"` // same value as Status.Uptime, duplicated for convenience
	LoadAvg    *LoadAvg        `json:"loadavg"`
	CPU        *CPUInfo        `json:"cpu"`
	Memory     *MemoryInfo     `json:"memory"`
	Disk       *DiskInfo       `json:"disk"`
	Interfaces []InterfaceInfo `json:"interfaces"`
	GatewayV4  *string         `json:"gateway_v4"` // default route nexthop, from `ip -j route`
	GatewayV6  *string         `json:"gateway_v6"` // default route nexthop, from `ip -6 -j route`
}

// LoadAvg mirrors the first three fields of /proc/loadavg.
type LoadAvg struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// CPUInfo summarizes /proc/cpuinfo plus the hottest thermal zone under
// /sys/class/thermal. TemperatureC is nil when no thermal zone is present.
type CPUInfo struct {
	Model        string   `json:"model"`
	Cores        int      `json:"cores"`
	TemperatureC *float64 `json:"temperature_c"`
}

// MemoryInfo mirrors the relevant fields of /proc/meminfo, converted to bytes.
type MemoryInfo struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

// DiskInfo is the result of statfs("/") converted to bytes.
type DiskInfo struct {
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

// InterfaceInfo is one row of the dashboard's "active connections" table.
type InterfaceInfo struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"` // bridge | wireless | ppp | ethernet
	MAC           string   `json:"mac"`  // /sys/class/net/<name>/address
	IPv4          []string `json:"ipv4"`
	IPv6          []string `json:"ipv6"`
	State         string   `json:"state"`           // /sys/class/net/<name>/operstate
	LinkSpeedMbps *int     `json:"link_speed_mbps"` // /sys/class/net/<name>/speed; nil when down/virtual/unreadable
}

// AddrInfo mirrors one entry from `ip -j addr`.
type AddrInfo struct {
	Ifname    string      `json:"ifname"`
	Flags     []string    `json:"flags"`
	AddrInfos []IPAddress `json:"addr_info"`
}

type IPAddress struct {
	Family    string `json:"family"`
	Local     string `json:"local"`
	Prefixlen int    `json:"prefixlen"`
	Scope     string `json:"scope"`
}

// RouteInfo mirrors one entry from `ip -j route`.
type RouteInfo struct {
	Dst      string `json:"dst"`
	Gateway  string `json:"gateway"`
	Dev      string `json:"dev"`
	Protocol string `json:"protocol"`
}

// DHCPLease is one line from /var/lib/misc/dnsmasq.leases.
type DHCPLease struct {
	Expiry   int64  `json:"expiry"` // Unix timestamp
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// ServiceInfo is the result of `systemctl is-active <unit>`.
type ServiceInfo struct {
	Name   string `json:"name"`
	Active string `json:"active"` // "active", "inactive", "failed", "unknown"
}

var monitoredServices = []string{
	"systemd-networkd",
	"dnsmasq",
	"smartdns",
	"nftables",
	"caddy",
	"pppd",
}

// Collect gathers all status fields. Errors inside sub-collectors are logged and
// the affected field is left nil / empty — they do not abort the response.
func Collect(ctx context.Context, leasesFile string) Status {
	up := uptime()
	addrs := ipAddrs(ctx)
	routes := ipRoutes(ctx)
	return Status{
		Uptime:   up,
		Addrs:    addrs,
		Routes:   routes,
		Nftables: nftRuleset(ctx),
		Leases:   parseDNSMasqLeases(leasesFile),
		Services: serviceStates(ctx),
		System:   collectSystem(ctx, addrs, routes, up),
	}
}

// ServiceState returns the systemctl is-active output for the given unit.
// Exported for reuse by the health package's service checks.
func ServiceState(ctx context.Context, unit string) string {
	return serviceState(ctx, unit)
}

func collectSystem(ctx context.Context, addrs []AddrInfo, routesV4 []RouteInfo, up string) SystemInfo {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	return SystemInfo{
		Hostname:   hostname,
		OS:         osPrettyName(),
		Kernel:     runTrim(ctx, "uname", "-r"),
		Arch:       runTrim(ctx, "uname", "-m"),
		Uptime:     up,
		LoadAvg:    loadAvg(),
		CPU:        cpuInfo(),
		Memory:     memoryInfo(),
		Disk:       diskInfo(),
		Interfaces: interfaces(addrs),
		GatewayV4:  defaultGateway(routesV4),
		GatewayV6:  defaultGateway(ipRoutes6(ctx)),
	}
}

// defaultGateway returns the nexthop of the default route (dst == "default"
// with a non-empty gateway), or nil if there isn't one.
func defaultGateway(routes []RouteInfo) *string {
	for _, r := range routes {
		if r.Dst == "default" && r.Gateway != "" {
			gw := r.Gateway
			return &gw
		}
	}
	return nil
}

// osPrettyName reads PRETTY_NAME from /etc/os-release. Returns nil if the
// file is absent (e.g. macOS dev) or the key isn't present.
func osPrettyName() *string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "PRETTY_NAME=") {
			continue
		}
		v := strings.TrimPrefix(line, "PRETTY_NAME=")
		v = strings.Trim(v, `"`)
		return &v
	}
	return nil
}

// runTrim runs a command and returns its trimmed stdout, or nil on any error
// (missing binary, non-zero exit, timeout).
func runTrim(ctx context.Context, name string, args ...string) *string {
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(c, name, args...).Output()
	if err != nil {
		return nil
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return nil
	}
	return &v
}

// loadAvg parses /proc/loadavg. Returns nil when the file is absent.
func loadAvg() *LoadAvg {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil
	}
	l1, err1 := strconv.ParseFloat(fields[0], 64)
	l5, err5 := strconv.ParseFloat(fields[1], 64)
	l15, err15 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err5 != nil || err15 != nil {
		return nil
	}
	return &LoadAvg{Load1: l1, Load5: l5, Load15: l15}
}

// cpuInfo reads /proc/cpuinfo for model name and core count, and the
// hottest thermal zone under /sys/class/thermal for temperature. Returns
// nil only when /proc/cpuinfo itself is unreadable (e.g. macOS dev).
func cpuInfo() *CPUInfo {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return nil
	}
	defer f.Close()

	var model string
	var cores int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case model == "" && strings.HasPrefix(line, "model name"):
			if idx := strings.Index(line, ":"); idx >= 0 {
				model = strings.TrimSpace(line[idx+1:])
			}
		case strings.HasPrefix(line, "processor"):
			cores++
		}
	}
	return &CPUInfo{
		Model:        model,
		Cores:        cores,
		TemperatureC: thermalZoneMax(),
	}
}

// thermalZoneMax returns the maximum temperature (Celsius) across all
// /sys/class/thermal/thermal_zone*/temp files, or nil if none are present.
func thermalZoneMax() *float64 {
	paths, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	if err != nil || len(paths) == 0 {
		return nil
	}
	var max float64
	found := false
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		milliC, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			continue
		}
		c := float64(milliC) / 1000.0
		if !found || c > max {
			max = c
			found = true
		}
	}
	if !found {
		return nil
	}
	return &max
}

// memoryInfo parses /proc/meminfo. Returns nil when the file is absent.
func memoryInfo() *MemoryInfo {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer f.Close()

	var total, avail uint64
	var haveTotal, haveAvail bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				total = v * 1024
				haveTotal = true
			}
		case "MemAvailable:":
			if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				avail = v * 1024
				haveAvail = true
			}
		}
	}
	if !haveTotal || !haveAvail {
		return nil
	}
	return &MemoryInfo{TotalBytes: total, AvailableBytes: avail}
}

// diskInfo statfs's "/" for total/free bytes. Works on both Linux and
// Darwin (dev), since syscall.Statfs_t exposes Bsize/Blocks/Bavail on both,
// just with differing underlying integer widths.
func diskInfo() *DiskInfo {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return nil
	}
	bsize := uint64(st.Bsize)
	return &DiskInfo{
		TotalBytes: bsize * uint64(st.Blocks),
		FreeBytes:  bsize * uint64(st.Bavail),
	}
}

// interfaces derives the dashboard's "active connections" rows from the
// already-fetched `ip -j addr` snapshot, classifying each interface's type
// by inspecting /sys/class/net/<name> and reading its operstate. Loopback
// is excluded.
func interfaces(addrs []AddrInfo) []InterfaceInfo {
	var out []InterfaceInfo
	for _, a := range addrs {
		if a.Ifname == "lo" {
			continue
		}
		var ipv4, ipv6 []string
		for _, ai := range a.AddrInfos {
			switch ai.Family {
			case "inet":
				ipv4 = append(ipv4, ai.Local)
			case "inet6":
				ipv6 = append(ipv6, ai.Local)
			}
		}
		out = append(out, InterfaceInfo{
			Name:          a.Ifname,
			Type:          ifaceType(a.Ifname),
			MAC:           macAddress(a.Ifname),
			IPv4:          ipv4,
			IPv6:          ipv6,
			State:         operState(a.Ifname),
			LinkSpeedMbps: linkSpeedMbps(a.Ifname),
		})
	}
	return out
}

// macAddress reads /sys/class/net/<name>/address. Returns "" if unreadable.
func macAddress(name string) string {
	data, err := os.ReadFile("/sys/class/net/" + name + "/address")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// linkSpeedMbps reads /sys/class/net/<name>/speed. The kernel reports -1 (or
// the read fails outright) when the link is down or the driver doesn't
// support reporting speed (e.g. most virtual interfaces) — both cases
// normalize to nil so the frontend can render "unknown" instead of a bogus value.
func linkSpeedMbps(name string) *int {
	data, err := os.ReadFile("/sys/class/net/" + name + "/speed")
	if err != nil {
		return nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || v < 0 {
		return nil
	}
	return &v
}

func ifaceType(name string) string {
	base := "/sys/class/net/" + name
	if dirExists(base + "/bridge") {
		return "bridge"
	}
	if dirExists(base + "/wireless") {
		return "wireless"
	}
	if strings.HasPrefix(name, "ppp") {
		return "ppp"
	}
	return "ethernet"
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func operState(name string) string {
	data, err := os.ReadFile("/sys/class/net/" + name + "/operstate")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// uptime returns the system uptime as a human-readable string (e.g. "123456.78s").
func uptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return ""
	}
	parts := strings.Fields(string(data))
	if len(parts) == 0 {
		return ""
	}
	return parts[0] + "s"
}

func ipAddrs(ctx context.Context) []AddrInfo {
	out, err := runJSON(ctx, "ip", "-j", "addr")
	if err != nil {
		return nil
	}
	var addrs []AddrInfo
	if err := json.Unmarshal(out, &addrs); err != nil {
		slog.Warn("parse ip -j addr", "err", err)
		return nil
	}
	return addrs
}

func ipRoutes(ctx context.Context) []RouteInfo {
	out, err := runJSON(ctx, "ip", "-j", "route")
	if err != nil {
		return nil
	}
	var routes []RouteInfo
	if err := json.Unmarshal(out, &routes); err != nil {
		slog.Warn("parse ip -j route", "err", err)
		return nil
	}
	return routes
}

// ipRoutes6 fetches the IPv6 routing table separately — `ip -j route` (no
// -6) only returns the IPv4 main table, so the IPv6 default gateway needs
// its own command. Used only to derive SystemInfo.GatewayV6; not exposed as
// a top-level Status field.
func ipRoutes6(ctx context.Context) []RouteInfo {
	out, err := runJSON(ctx, "ip", "-6", "-j", "route")
	if err != nil {
		return nil
	}
	var routes []RouteInfo
	if err := json.Unmarshal(out, &routes); err != nil {
		slog.Warn("parse ip -6 -j route", "err", err)
		return nil
	}
	return routes
}

// nftRuleset returns the raw JSON from `nft -j list ruleset`.
// The JSON is forwarded as-is to avoid re-serializing the complex nft schema.
func nftRuleset(ctx context.Context) json.RawMessage {
	out, err := runJSON(ctx, "nft", "-j", "list", "ruleset")
	if err != nil {
		return nil
	}
	if !json.Valid(out) {
		slog.Warn("nft -j list ruleset: invalid JSON")
		return nil
	}
	return json.RawMessage(out)
}

// parseDNSMasqLeases parses /var/lib/misc/dnsmasq.leases.
// Format: <expiry> <mac> <ip> <hostname> <client-id>
// ParseLeasesFile is exported for testing.
func ParseLeasesFile(path string) []DHCPLease {
	return parseDNSMasqLeases(path)
}

func parseDNSMasqLeases(path string) []DHCPLease {
	if path == "" {
		path = "/var/lib/misc/dnsmasq.leases"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var leases []DHCPLease
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		var expiry int64
		for _, b := range []byte(fields[0]) {
			if b < '0' || b > '9' {
				expiry = 0
				break
			}
			expiry = expiry*10 + int64(b-'0')
		}
		hostname := fields[3]
		if hostname == "*" {
			hostname = ""
		}
		leases = append(leases, DHCPLease{
			Expiry:   expiry,
			MAC:      fields[1],
			IP:       fields[2],
			Hostname: hostname,
		})
	}
	return leases
}

func serviceStates(ctx context.Context) []ServiceInfo {
	var infos []ServiceInfo
	for _, svc := range monitoredServices {
		state := serviceState(ctx, svc)
		infos = append(infos, ServiceInfo{Name: svc, Active: state})
	}
	return infos
}

func serviceState(ctx context.Context, unit string) string {
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, "systemctl", "is-active", unit)
	out, err := cmd.Output()
	if err != nil {
		// systemctl exits non-zero for "inactive" or "failed" — that's still a valid state
		if len(out) > 0 {
			return strings.TrimSpace(string(out))
		}
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func runJSON(ctx context.Context, name string, args ...string) ([]byte, error) {
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("command failed", "cmd", name, "args", args, "err", err)
		return nil, err
	}
	return out, nil
}
