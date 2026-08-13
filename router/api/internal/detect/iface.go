package detect

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// NetPath is the sysfs base directory for network interfaces.
// Overridden in tests to point at fixture directories.
var NetPath = "/sys/class/net"

// UdevadmBin is the udevadm binary used to query udev properties.
// Overridden in tests or when udevadm is absent.
var UdevadmBin = "udevadm"

// Interface describes a network interface detected from sysfs.
type Interface struct {
	Name   string
	Path   string // udev ID_PATH value; empty when not available
	MAC    string
	LinkUp bool // true when operstate == "up"
	Wifi   bool // true when /sys/class/net/<name>/wireless/ exists
}

// EnumerateInterfaces reads /sys/class/net and returns all detected
// network interfaces (excluding loopback and bridge devices). Wifi
// interfaces are included and identified by Interface.Wifi = true.
// Degrades gracefully on non-Linux: returns an empty slice without error
// when NetPath does not exist.
func EnumerateInterfaces() ([]Interface, error) {
	entries, err := os.ReadDir(NetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // non-Linux / no sysfs
		}
		return nil, err
	}

	var result []Interface
	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		// Skip bridge master devices (they have a bridge/ subdirectory).
		bridgeDir := filepath.Join(NetPath, name, "bridge")
		if _, err := os.Stat(bridgeDir); err == nil {
			continue
		}
		iface := Interface{Name: name}
		iface.Wifi = isWifi(name)
		iface.MAC = readSysfsString(filepath.Join(NetPath, name, "address"))
		iface.LinkUp = readSysfsString(filepath.Join(NetPath, name, "operstate")) == "up"
		iface.Path = udevIDPath(name)
		result = append(result, iface)
	}
	return result, nil
}

// isWifi reports whether the named interface has a wireless/ subdirectory,
// which the kernel creates for all 802.11 network interfaces.
func isWifi(name string) bool {
	_, err := os.Stat(filepath.Join(NetPath, name, "wireless"))
	return err == nil
}

// readSysfsString reads a sysfs attribute file, trims whitespace, and returns
// the result. Returns "" on any error.
func readSysfsString(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is always inside NetPath (a trusted sysfs tree), not user input
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n\r ")
}

// udevIDPath queries udevadm for the ID_PATH property of the given interface.
// Returns "" when udevadm is unavailable or the property is absent.
func udevIDPath(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sysPath := filepath.Join(NetPath, name)
	cmd := exec.CommandContext(ctx, UdevadmBin, "info", "-q", "property", "-p", sysPath)
	out, err := cmd.Output()
	if err != nil {
		return "" // udevadm absent or interface has no udev record
	}
	return parseIDPath(out)
}

// parseIDPath extracts the ID_PATH value from udevadm property output.
// Each line is "KEY=value"; we look for "ID_PATH=".
func parseIDPath(data []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "ID_PATH=") {
			return strings.TrimPrefix(line, "ID_PATH=")
		}
	}
	return ""
}
