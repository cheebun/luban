package apply

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyFilePreserveMode_KeepsSourceMode guards against a regression where
// installRendered's backup step (and Rollback's restore step) hardcoded
// copyFile()'s 0644 default, silently downgrading chap-secrets from 0600 to
// world-readable and leaking plaintext PPPoE credentials via the .bak file.
func TestCopyFilePreserveMode_KeepsSourceMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "chap-secrets")
	dst := filepath.Join(dir, "chap-secrets.bak")

	if err := os.WriteFile(src, []byte("user * secret *\n"), 0600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := copyFilePreserveMode(src, dst); err != nil {
		t.Fatalf("copyFilePreserveMode: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("dst mode = %o, want 0600", got)
	}

	// Round-trip: restoring from the 0600 .bak must also land back at 0600,
	// not fall back to the copyFile() 0644 default.
	restored := filepath.Join(dir, "chap-secrets.restored")
	if err := copyFilePreserveMode(dst, restored); err != nil {
		t.Fatalf("copyFilePreserveMode restore: %v", err)
	}
	info, err = os.Stat(restored)
	if err != nil {
		t.Fatalf("stat restored: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("restored mode = %o, want 0600", got)
	}
}
