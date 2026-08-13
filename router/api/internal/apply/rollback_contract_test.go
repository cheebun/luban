package apply

// TestRollbackScriptContract guards against the Go apply pipeline and the
// shell rollback script (router-rollback.sh, run by router-rollback.timer
// for the power-loss / 90s-expiry anti-lockout path) drifting onto two
// different backup schemes again. Both sides must agree that the backup of
// each live generated file is "<OutputPath>.bak" beside it — see
// installRendered/Rollback in pipeline.go and the restore section of
// router-rollback.sh.
//
// It is repo-layout dependent (walks up from this package to find
// router/systemd/), so it skips cleanly if that directory isn't present.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func systemdDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "systemd")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("router/systemd/ not found at %s (repo layout unavailable): %v", dir, err)
	}
	return dir
}

func TestRollbackScriptContract(t *testing.T) {
	dir := systemdDir(t)

	scriptPath := filepath.Join(dir, "router-rollback.sh")
	scriptBytes, err := os.ReadFile(scriptPath) //nolint:gosec // G304: path is derived from a test-only helper that resolves a known sibling directory
	if err != nil {
		t.Skipf("router-rollback.sh not found at %s: %v", scriptPath, err)
	}
	script := string(scriptBytes)

	// (a) Every static (non-networkd) renderTable OutputPath must appear
	// verbatim in the script — those are restored individually by exact
	// path. The .network/.netdev outputs are restored via the generic globs
	// checked in (b) instead, since their filenames vary (per-LAN-port).
	for _, e := range renderTable {
		if strings.HasSuffix(e.OutputPath, ".network") || strings.HasSuffix(e.OutputPath, ".netdev") {
			continue
		}
		if !strings.Contains(script, e.OutputPath) {
			t.Errorf("renderTable OutputPath %q not found verbatim in router-rollback.sh", e.OutputPath)
		}
	}

	// (b) The script must restore .network/.netdev backups via a glob, not
	// an enumerated/stale filename list.
	if !strings.Contains(script, ".network.bak") {
		t.Error("router-rollback.sh has no glob covering *.network.bak")
	}
	if !strings.Contains(script, ".netdev.bak") {
		t.Error("router-rollback.sh has no glob covering *.netdev.bak")
	}

	// (c) The retired unit name must not resurface anywhere in the Go source
	// (excluding this test file itself, which necessarily names it), and the
	// unit name Go does use must exist in router/systemd/.
	retiredUnit := "pppd" + "-wan" // split to avoid this test file self-matching (c)
	selfPath, _ := filepath.Abs("rollback_contract_test.go")
	apiDir := filepath.Join(dir, "..", "api")
	err = filepath.Walk(apiDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		absPath, absErr := filepath.Abs(path)
		if absErr == nil && absPath == selfPath {
			return nil
		}
		b, readErr := os.ReadFile(path) //nolint:gosec // G304: path comes from filepath.WalkDir over a trusted project directory
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(b), retiredUnit) {
			t.Errorf("%s: retired unit name %q found — should be %q", path, retiredUnit, "pppd")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", apiDir, err)
	}

	if _, err := os.Stat(filepath.Join(dir, "pppd.service")); err != nil {
		t.Errorf("pppd.service not found in %s: %v", dir, err)
	}
}
