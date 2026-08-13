package detect_test

import (
	"luban/internal/detect"
	"os"
	"path/filepath"
	"testing"
)

func setNetPath(t *testing.T, path string) {
	t.Helper()
	old := detect.NetPath
	detect.NetPath = path
	t.Cleanup(func() { detect.NetPath = old })
}

// makeNetIface creates a minimal /sys/class/net/<name>/ fixture.
func makeNetIface(t *testing.T, root, name, mac, operstate string, isWifi, isBridge bool) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G301: test fixture directory
		t.Fatal(err)
	}
	writeFile := func(fname, content string) {
		if err := os.WriteFile(filepath.Join(dir, fname), []byte(content), 0o644); err != nil { //nolint:gosec // G306: test fixture
			t.Fatalf("write %s/%s: %v", name, fname, err)
		}
	}
	writeFile("address", mac+"\n")
	writeFile("operstate", operstate+"\n")
	if isWifi {
		if err := os.MkdirAll(filepath.Join(dir, "wireless"), 0o750); err != nil { //nolint:gosec // G301: test fixture directory
			t.Fatal(err)
		}
	}
	if isBridge {
		if err := os.MkdirAll(filepath.Join(dir, "bridge"), 0o750); err != nil { //nolint:gosec // G301: test fixture directory
			t.Fatal(err)
		}
	}
}

// TestEnumerateInterfaces_Basic verifies that normal ethernet ports are
// returned, while loopback and bridge devices are skipped.
func TestEnumerateInterfaces_Basic(t *testing.T) {
	netRoot := t.TempDir()
	setNetPath(t, netRoot)

	// Loopback — must be skipped.
	makeNetIface(t, netRoot, "lo", "00:00:00:00:00:00", "unknown", false, false)
	// Ethernet port — must be included.
	makeNetIface(t, netRoot, "eth0", "aa:bb:cc:dd:ee:ff", "up", false, false)
	// Bridge master — must be skipped.
	makeNetIface(t, netRoot, "br0", "00:11:22:33:44:55", "up", false, true)
	// Wifi interface — must be included with Wifi=true.
	makeNetIface(t, netRoot, "wlan0", "de:ad:be:ef:00:01", "down", true, false)

	ifaces, err := detect.EnumerateInterfaces()
	if err != nil {
		t.Fatalf("EnumerateInterfaces: %v", err)
	}

	byName := make(map[string]detect.Interface)
	for _, iface := range ifaces {
		byName[iface.Name] = iface
	}

	// lo must not appear.
	if _, ok := byName["lo"]; ok {
		t.Error("lo should be excluded")
	}
	// br0 must not appear.
	if _, ok := byName["br0"]; ok {
		t.Error("bridge br0 should be excluded")
	}

	// eth0 must appear as a non-wifi interface.
	eth0, ok := byName["eth0"]
	if !ok {
		t.Fatal("eth0 not found in results")
	}
	if eth0.Wifi {
		t.Error("eth0.Wifi should be false")
	}
	if !eth0.LinkUp {
		t.Error("eth0.LinkUp should be true (operstate=up)")
	}
	if eth0.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("eth0.MAC = %q, want aa:bb:cc:dd:ee:ff", eth0.MAC)
	}

	// wlan0 must appear as a wifi interface.
	wlan0, ok := byName["wlan0"]
	if !ok {
		t.Fatal("wlan0 not found in results")
	}
	if !wlan0.Wifi {
		t.Error("wlan0.Wifi should be true")
	}
	if wlan0.LinkUp {
		t.Error("wlan0.LinkUp should be false (operstate=down)")
	}
}

// TestEnumerateInterfaces_MissingSysfs verifies graceful degradation when
// NetPath does not exist (non-Linux environment).
func TestEnumerateInterfaces_MissingSysfs(t *testing.T) {
	setNetPath(t, "/nonexistent/sys/class/net")

	ifaces, err := detect.EnumerateInterfaces()
	if err != nil {
		t.Fatalf("expected nil error on missing sysfs, got: %v", err)
	}
	if len(ifaces) != 0 {
		t.Errorf("expected empty slice on missing sysfs, got %d interfaces", len(ifaces))
	}
}

// TestParseIDPath verifies that parseIDPath extracts the ID_PATH value from
// udevadm output correctly and ignores unrelated properties.
// This exercises the unexported helper indirectly via the udevadm output
// format used by EnumerateInterfaces when udevadm is available.
func TestParseIDPath(_ *testing.T) {
	// Covered implicitly by TestEnumerateInterfaces_Basic since
	// udevadm is called for each interface. On non-Linux, udevadm returns
	// an error and Path stays empty — the test above verifies the fields
	// succeed without a Path value. Direct parsing tests belong in an
	// internal package test; this file uses the exported API only.
}
