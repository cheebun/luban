// Package health implements the router's self-check (dependencies/diagnostics) page:
// checking that required binaries are installed, that the services we
// depend on are active, and that the resolver/network-manager takeover
// this project relies on (masking systemd-resolved, disabling
// NetworkManager, etc.) is actually in place.
package health

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"luban/internal/status"
)

const cmdTimeout = 5 * time.Second

var errUnknownService = errors.New("service is not in the restart allowlist")

// Component is one row of the "components" section: a binary this
// project shells out to.
type Component struct {
	Name      string  `json:"name"`
	Installed bool    `json:"installed"`
	Version   *string `json:"version"`
	Detail    string  `json:"detail"`
}

// ServiceStatus is one row of the "services" section.
type ServiceStatus struct {
	Name        string `json:"name"`
	Active      string `json:"active"` // systemctl is-active output
	Detail      string `json:"detail"`
	Restartable bool   `json:"restartable"`
}

// TakeoverCheck is one row of the "takeover checks" section:
// verifies this project has successfully taken over DNS/network management
// from the distro defaults (systemd-resolved, NetworkManager).
type TakeoverCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Health is the full payload returned by GET /api/health.
type Health struct {
	Components []Component     `json:"components"`
	Services   []ServiceStatus `json:"services"`
	Takeover   []TakeoverCheck `json:"takeover"`
}

// checkedBinaries lists the external binaries this project depends on, and
// the flag used to probe each one's version. Order matches the reference
// UI's components table.
var checkedBinaries = []struct {
	name        string
	versionArgs []string
}{
	{"dnsmasq", []string{"--version"}},
	{"smartdns", []string{"-v"}},
	{"caddy", []string{"version"}},
	{"nft", []string{"--version"}},
	{"pppd", []string{"--version"}},
	{"networkctl", []string{"--version"}},
	{"iw", []string{"--version"}},
	{"curl", []string{"--version"}},
}

// RestartableServices is the hard allowlist for POST /api/service/restart.
// Any name not in this set is rejected outright — this list must never be
// derived from user input.
var RestartableServices = map[string]bool{
	"router-ui":         true,
	"caddy":             true,
	"dnsmasq":           true,
	"smartdns":          true,
	"systemd-networkd":  true,
	"nftables":          true,
	"systemd-timesyncd": true,
	"pppd":              true,
}

// RestartService runs `systemctl restart <name>` after verifying name is on
// RestartableServices. Callers (the HTTP handler) are responsible for the
// router-ui self-restart special case — see server.handleServiceRestart.
func RestartService(ctx context.Context, name string) error {
	if !RestartableServices[name] {
		return errUnknownService
	}
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	return exec.CommandContext(c, "systemctl", "restart", name).Run()
}

// monitoredServices lists every service shown in the services section, in the
// order the reference UI displays them.
var monitoredServices = []string{
	"router-ui",
	"caddy",
	"dnsmasq",
	"smartdns",
	"systemd-networkd",
	"nftables",
	"systemd-timesyncd",
	"pppd",
}

// Collect gathers all health sections. Every sub-check degrades gracefully
// (installed=false / active="unknown" / ok=false with a Detail explaining
// why) rather than failing the whole response — this endpoint must always
// answer, including on a macOS dev box with none of these binaries present.
func Collect(ctx context.Context) Health {
	return Health{
		Components: collectComponents(ctx),
		Services:   collectServices(ctx),
		Takeover:   collectTakeover(ctx),
	}
}

func collectComponents(ctx context.Context) []Component {
	out := make([]Component, 0, len(checkedBinaries))
	for _, b := range checkedBinaries {
		path, err := exec.LookPath(b.name)
		if err != nil {
			out = append(out, Component{
				Name:      b.name,
				Installed: false,
				Detail:    "未找到可执行文件",
			})
			continue
		}
		version := probeVersion(ctx, b.name, b.versionArgs)
		detail := path
		out = append(out, Component{
			Name:      b.name,
			Installed: true,
			Version:   version,
			Detail:    detail,
		})
	}
	return out
}

// probeVersion runs `<bin> <args>` and returns the first line of stdout
// (falling back to stderr, since some tools like pppd print version info
// there), trimmed. Returns nil if the probe fails or produces no output —
// the binary is still reported as installed, just with an unknown version.
func probeVersion(ctx context.Context, bin string, args []string) *string {
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil
	}
	line := strings.TrimSpace(firstLine(string(out)))
	if line == "" {
		return nil
	}
	return &line
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func collectServices(ctx context.Context) []ServiceStatus {
	out := make([]ServiceStatus, 0, len(monitoredServices))
	for _, name := range monitoredServices {
		active := status.ServiceState(ctx, name)
		detail := ""
		if active == "unknown" {
			detail = "systemctl 不可用或该 unit 不存在"
		}
		out = append(out, ServiceStatus{
			Name:        name,
			Active:      active,
			Detail:      detail,
			Restartable: RestartableServices[name],
		})
	}
	return out
}

func collectTakeover(ctx context.Context) []TakeoverCheck {
	return []TakeoverCheck{
		checkResolvedMasked(ctx),
		checkNetworkdEnabled(ctx),
		checkResolvConf(),
		checkNetworkManagerAbsent(ctx),
	}
}

func checkResolvedMasked(ctx context.Context) TakeoverCheck {
	state := isEnabledState(ctx, "systemd-resolved")
	if state == "masked" {
		return TakeoverCheck{Name: "systemd-resolved 已屏蔽", OK: true, Detail: "masked"}
	}
	return TakeoverCheck{
		Name:   "systemd-resolved 已屏蔽",
		OK:     false,
		Detail: "当前状态: " + state + "（应为 masked，否则会与 dnsmasq/smartdns 抢占 53 端口）",
	}
}

func checkNetworkdEnabled(ctx context.Context) TakeoverCheck {
	state := isEnabledState(ctx, "systemd-networkd")
	if state == "enabled" {
		return TakeoverCheck{Name: "systemd-networkd 已启用", OK: true, Detail: "enabled"}
	}
	return TakeoverCheck{
		Name:   "systemd-networkd 已启用",
		OK:     false,
		Detail: "当前状态: " + state,
	}
}

func checkResolvConf() TakeoverCheck {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return TakeoverCheck{Name: "/etc/resolv.conf 指向本机解析器", OK: false, Detail: "读取失败: " + err.Error()}
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "nameserver" && fields[1] == "127.0.0.1" {
			return TakeoverCheck{Name: "/etc/resolv.conf 指向本机解析器", OK: true, Detail: "nameserver 127.0.0.1"}
		}
	}
	return TakeoverCheck{
		Name:   "/etc/resolv.conf 指向本机解析器",
		OK:     false,
		Detail: "未找到 nameserver 127.0.0.1",
	}
}

func checkNetworkManagerAbsent(ctx context.Context) TakeoverCheck {
	if _, err := exec.LookPath("NetworkManager"); err != nil {
		return TakeoverCheck{Name: "NetworkManager 未接管网络", OK: true, Detail: "未安装"}
	}
	state := isEnabledState(ctx, "NetworkManager")
	if state == "disabled" || state == "masked" || state == "not-found" {
		return TakeoverCheck{Name: "NetworkManager 未接管网络", OK: true, Detail: "已安装但 " + state}
	}
	return TakeoverCheck{
		Name:   "NetworkManager 未接管网络",
		OK:     false,
		Detail: "当前状态: " + state + "，可能与 systemd-networkd 冲突",
	}
}

// isEnabledState runs `systemctl is-enabled <unit>` and returns its trimmed
// output. Non-zero exit codes from systemctl (e.g. "masked", "disabled")
// still carry a meaningful state in stdout, so those are returned as-is
// rather than treated as failure. "unknown" is only returned when systemctl
// itself is unavailable or produced no output at all.
func isEnabledState(ctx context.Context, unit string) string {
	c, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(c, "systemctl", "is-enabled", unit).Output()
	if err != nil {
		if len(out) > 0 {
			return strings.TrimSpace(string(out))
		}
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
