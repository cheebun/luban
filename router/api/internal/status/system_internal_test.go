package status

import (
	"context"
	"testing"
)

// TestInterfaces_DerivesIPsAndExcludesLoopback exercises the pure
// transformation from an `ip -j addr`-shaped fixture to the dashboard's
// InterfaceInfo rows, without touching the filesystem for the IP data
// itself (only ifaceType/operState hit /sys, and degrade to "ethernet"/
// "unknown" off-box, which this test tolerates).
func TestInterfaces_DerivesIPsAndExcludesLoopback(t *testing.T) {
	fixture := []AddrInfo{
		{
			Ifname: "lo",
			AddrInfos: []IPAddress{
				{Family: "inet", Local: "127.0.0.1"},
			},
		},
		{
			Ifname: "eth0",
			AddrInfos: []IPAddress{
				{Family: "inet", Local: "192.168.1.1"},
				{Family: "inet6", Local: "fe80::1"},
			},
		},
		{
			Ifname:    "ppp0",
			AddrInfos: []IPAddress{{Family: "inet", Local: "10.0.0.2"}},
		},
	}

	out := interfaces(fixture)

	if len(out) != 2 {
		t.Fatalf("expected loopback excluded (2 remaining), got %d: %+v", len(out), out)
	}

	byName := map[string]InterfaceInfo{}
	for _, i := range out {
		byName[i.Name] = i
	}

	eth0, ok := byName["eth0"]
	if !ok {
		t.Fatal("expected eth0 in output")
	}
	if len(eth0.IPv4) != 1 || eth0.IPv4[0] != "192.168.1.1" {
		t.Errorf("eth0.IPv4 = %v; want [192.168.1.1]", eth0.IPv4)
	}
	if len(eth0.IPv6) != 1 || eth0.IPv6[0] != "fe80::1" {
		t.Errorf("eth0.IPv6 = %v; want [fe80::1]", eth0.IPv6)
	}

	ppp0, ok := byName["ppp0"]
	if !ok {
		t.Fatal("expected ppp0 in output")
	}
	if ppp0.Type != "ppp" {
		t.Errorf("ppp0.Type = %q; want %q (inferred from name prefix)", ppp0.Type, "ppp")
	}
}

func TestInterfaces_EmptyInput(t *testing.T) {
	if out := interfaces(nil); out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
}

// TestLinkSpeedMbps_MissingInterfaceIsNil covers the degrade-to-nil path for
// a nonexistent interface (same behavior as a down/virtual NIC on real
// hardware, where /sys/class/net/<name>/speed reads -1 or errors).
func TestLinkSpeedMbps_MissingInterfaceIsNil(t *testing.T) {
	if v := linkSpeedMbps("definitely-not-a-real-nic"); v != nil {
		t.Errorf("expected nil for a nonexistent interface, got %v", *v)
	}
}

func TestMacAddress_MissingInterfaceIsEmpty(t *testing.T) {
	if mac := macAddress("definitely-not-a-real-nic"); mac != "" {
		t.Errorf("expected empty string for a nonexistent interface, got %q", mac)
	}
}

func TestDefaultGateway(t *testing.T) {
	routes := []RouteInfo{
		{Dst: "192.168.1.0/24", Dev: "eth0"},
		{Dst: "default", Gateway: "192.168.1.1", Dev: "eth0"},
	}
	gw := defaultGateway(routes)
	if gw == nil || *gw != "192.168.1.1" {
		t.Errorf("defaultGateway = %v; want 192.168.1.1", gw)
	}
}

func TestDefaultGateway_NoneFound(t *testing.T) {
	routes := []RouteInfo{{Dst: "192.168.1.0/24", Dev: "eth0"}}
	if gw := defaultGateway(routes); gw != nil {
		t.Errorf("expected nil default gateway, got %v", *gw)
	}
	if gw := defaultGateway(nil); gw != nil {
		t.Errorf("expected nil default gateway for empty routes, got %v", *gw)
	}
}

// TestCollectSystem_NeverPanics is the "degrades gracefully on macOS"
// contract: every optional field must be nil rather than the call
// panicking or erroring when /proc and /sys don't exist.
func TestCollectSystem_NeverPanics(t *testing.T) {
	sys := collectSystem(context.Background(), nil, nil, "123.45s")
	if sys.Hostname == "" {
		t.Error("expected os.Hostname() to succeed even off-Linux")
	}
	if sys.Uptime != "123.45s" {
		t.Errorf("Uptime = %q; want %q", sys.Uptime, "123.45s")
	}
	// CPU/Memory/Disk/LoadAvg/OS/Kernel/Arch may legitimately be nil off-Linux;
	// the only requirement is that collectSystem returns rather than panicking,
	// which reaching this line already proves.
}
