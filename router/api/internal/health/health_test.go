package health_test

import (
	"context"
	"luban/internal/health"
	"testing"
	"time"
)

// TestCollect_DegradesGracefully exercises the real Collect() path — which
// is what /api/health calls — on whatever machine runs the test (macOS in
// dev, Linux in CI/prod). It must never panic or hang, and every section
// must be populated with the same length as its check list regardless of
// which binaries/services/checks are actually present.
func TestCollect_DegradesGracefully(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	h := health.Collect(ctx)

	if len(h.Components) == 0 {
		t.Fatal("expected a non-empty components list")
	}
	for _, c := range h.Components {
		if c.Name == "" {
			t.Errorf("component with empty name: %+v", c)
		}
		if !c.Installed && c.Version != nil {
			t.Errorf("component %q: not installed but has a version", c.Name)
		}
	}

	if len(h.Services) == 0 {
		t.Fatal("expected a non-empty services list")
	}
	for _, s := range h.Services {
		if s.Active == "" {
			t.Errorf("service %q: Active must never be empty (use \"unknown\")", s.Name)
		}
	}

	if len(h.Takeover) == 0 {
		t.Fatal("expected a non-empty takeover checks list")
	}
	for _, c := range h.Takeover {
		if c.Name == "" || c.Detail == "" {
			t.Errorf("takeover check with empty name/detail: %+v", c)
		}
	}
}

func TestRestartService_RejectsUnknownName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := health.RestartService(ctx, "totally-not-a-real-service"); err == nil {
		t.Fatal("expected RestartService to reject a name outside the allowlist")
	}
}

func TestRestartService_RejectsEmptyName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := health.RestartService(ctx, ""); err == nil {
		t.Fatal("expected RestartService to reject an empty name")
	}
}

// TestRestartableServices_MatchesReferenceList locks in the exact allowlist
// so an accidental addition/removal fails loudly instead of silently
// widening or narrowing what POST /api/service/restart accepts.
func TestRestartableServices_MatchesReferenceList(t *testing.T) {
	want := []string{
		"router-ui",
		"caddy",
		"dnsmasq",
		"smartdns",
		"systemd-networkd",
		"nftables",
		"systemd-timesyncd",
		"pppd",
	}
	if len(health.RestartableServices) != len(want) {
		t.Fatalf("RestartableServices has %d entries; want %d", len(health.RestartableServices), len(want))
	}
	for _, name := range want {
		if !health.RestartableServices[name] {
			t.Errorf("expected %q to be restartable", name)
		}
	}
}
