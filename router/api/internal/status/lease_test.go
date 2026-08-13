package status_test

import (
	"os"
	"path/filepath"
	"testing"

	"luban/internal/status"
)

func TestParseDNSMasqLeases(t *testing.T) {
	content := `1893456000 aa:bb:cc:dd:ee:ff 192.168.20.101 myphone *
1893456001 11:22:33:44:55:66 192.168.20.102 * *
# this is a comment
1893456002 de:ad:be:ef:00:01 192.168.20.103 laptop *
`
	dir := t.TempDir()
	path := filepath.Join(dir, "dnsmasq.leases")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	leases := status.ParseLeasesFile(path)
	if len(leases) != 3 {
		t.Fatalf("expected 3 leases, got %d", len(leases))
	}

	if leases[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("lease[0].MAC = %q; want aa:bb:cc:dd:ee:ff", leases[0].MAC)
	}
	if leases[0].IP != "192.168.20.101" {
		t.Errorf("lease[0].IP = %q; want 192.168.20.101", leases[0].IP)
	}
	if leases[0].Hostname != "myphone" {
		t.Errorf("lease[0].Hostname = %q; want myphone", leases[0].Hostname)
	}
	if leases[0].Expiry != 1893456000 {
		t.Errorf("lease[0].Expiry = %d; want 1893456000", leases[0].Expiry)
	}

	// Asterisk hostname should be normalized to ""
	if leases[1].Hostname != "" {
		t.Errorf("lease[1].Hostname = %q; want empty string", leases[1].Hostname)
	}
}

func TestParseDNSMasqLeases_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dnsmasq.leases")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	leases := status.ParseLeasesFile(path)
	if leases != nil && len(leases) != 0 {
		t.Errorf("expected nil/empty slice for empty file, got %v", leases)
	}
}

func TestParseDNSMasqLeases_Missing(t *testing.T) {
	leases := status.ParseLeasesFile("/nonexistent/path/dnsmasq.leases")
	if leases != nil {
		t.Errorf("expected nil for missing file, got %v", leases)
	}
}
