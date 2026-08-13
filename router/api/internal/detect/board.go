// Package detect provides first-boot hardware detection: board profile matching,
// ethernet interface enumeration, and DHCP offer probing.
package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DMIPath is the sysfs base directory for DMI ID files (x86 boards).
// Overridden in tests to point at fixture directories.
var DMIPath = "/sys/class/dmi/id"

// DTModelPath is the path to the device-tree model string (ARM boards).
// Overridden in tests to point at fixture files.
var DTModelPath = "/proc/device-tree/model"

// DMIMatch specifies the DMI fields required to match an x86 board.
// Only non-empty fields are compared; empty fields act as wildcards.
type DMIMatch struct {
	SysVendor     string `json:"sys_vendor,omitempty"`
	ProductName   string `json:"product_name,omitempty"`
	ProductFamily string `json:"product_family,omitempty"`
	BoardName     string `json:"board_name,omitempty"`
}

// Match groups the possible match strategies for a board profile.
// Exactly one of DMI or DTModel should be set per profile.
type Match struct {
	DMI     *DMIMatch `json:"dmi,omitempty"`
	DTModel string    `json:"dt_model,omitempty"`
}

// InterfaceSpec describes one physical network port in a board profile.
type InterfaceSpec struct {
	// Role is "wan", "lan", or "wifi".
	Role string `json:"role"`
	// Path is the udev ID_PATH value that uniquely identifies this port by bus
	// position across all units of the same board model.
	Path  string `json:"path"`
	Model string `json:"model,omitempty"`
	Label string `json:"label,omitempty"`
	Notes string `json:"notes,omitempty"`
}

// BoardProfile describes a known hardware model with its interface roles.
type BoardProfile struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Match      Match           `json:"match"`
	Interfaces []InterfaceSpec `json:"interfaces"`
}

// LoadBoards reads board profiles from boardsDir (boards/*.json) and
// boardsDDir (boards.d/*.json). Profiles from boards.d take precedence:
// they are placed first in the returned slice so MatchBoard finds them
// first. Missing directories are silently skipped.
func LoadBoards(boardsDir string) ([]BoardProfile, error) {
	dDir := boardsDir + ".d"
	var profiles []BoardProfile

	// boards.d/ first so user overrides take precedence over shipped profiles.
	if dProfiles, err := loadDir(dDir); err == nil {
		profiles = append(profiles, dProfiles...)
	}
	if sProfiles, err := loadDir(boardsDir); err == nil {
		profiles = append(profiles, sProfiles...)
	}
	return profiles, nil
}

func loadDir(dir string) ([]BoardProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var profiles []BoardProfile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: path is inside a trusted boards directory, not user input
		if err != nil {
			continue
		}
		var p BoardProfile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// MatchBoard iterates profiles and returns the first one whose match criteria
// are satisfied by the running hardware. Returns nil (no error) when no profile
// matches. Returns an error only on unexpected I/O failures.
func MatchBoard(profiles []BoardProfile) (*BoardProfile, error) {
	dmiValues := readDMI()
	dtModel := readDTModel()

	for i := range profiles {
		p := &profiles[i]
		if p.Match.DMI != nil {
			if matchDMI(p.Match.DMI, dmiValues) {
				return p, nil
			}
		}
		if p.Match.DTModel != "" {
			if strings.Contains(dtModel, p.Match.DTModel) {
				return p, nil
			}
		}
	}
	return nil, nil
}

// dmiField maps DMI match field names to their sysfs file names.
var dmiField = map[string]string{
	"sys_vendor":     "sys_vendor",
	"product_name":   "product_name",
	"product_family": "product_family",
	"board_name":     "board_name",
}

// dmiValues is the set of raw DMI sysfs values read once per MatchBoard call.
type dmiValues struct {
	SysVendor     string
	ProductName   string
	ProductFamily string
	BoardName     string
}

func readDMI() dmiValues {
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(DMIPath, name)) //nolint:gosec // G304: DMIPath is a package-level var pointing to /sys/class/dmi/id or a test fixture
		if err != nil {
			return ""
		}
		return strings.TrimRight(string(data), "\n\r ")
	}
	return dmiValues{
		SysVendor:     read(dmiField["sys_vendor"]),
		ProductName:   read(dmiField["product_name"]),
		ProductFamily: read(dmiField["product_family"]),
		BoardName:     read(dmiField["board_name"]),
	}
}

func readDTModel() string {
	data, err := os.ReadFile(DTModelPath) //nolint:gosec // G304: DTModelPath is a package-level var pointing to /proc/device-tree/model or a test fixture
	if err != nil {
		return ""
	}
	// device-tree model strings are NUL-terminated; strip the NUL.
	return strings.TrimRight(string(data), "\x00\n\r ")
}

// matchDMI returns true when every non-empty field in m equals the
// corresponding sysfs value. An empty m field is a wildcard (don't-care).
func matchDMI(m *DMIMatch, v dmiValues) bool {
	if m.SysVendor != "" && m.SysVendor != v.SysVendor {
		return false
	}
	if m.ProductName != "" && m.ProductName != v.ProductName {
		return false
	}
	if m.ProductFamily != "" && m.ProductFamily != v.ProductFamily {
		return false
	}
	if m.BoardName != "" && m.BoardName != v.BoardName {
		return false
	}
	return true
}
