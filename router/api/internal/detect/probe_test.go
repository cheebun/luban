package detect_test

import (
	"context"
	"errors"
	"luban/internal/detect"
	"testing"
)

// TestFakeProber_GotOffer verifies that FakeProber returns the configured
// results and marks an interface as having received a DHCP offer.
func TestFakeProber_GotOffer(t *testing.T) {
	prober := &detect.FakeProber{
		Results: map[string]detect.ProbeResult{
			"eth0": {Interface: "eth0", GotOffer: true, OfferServer: "192.168.1.1"},
			"eth1": {Interface: "eth1", GotOffer: false},
		},
	}

	results, err := prober.Probe(context.Background(), []string{"eth0", "eth1"})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	byName := make(map[string]detect.ProbeResult)
	for _, r := range results {
		byName[r.Interface] = r
	}

	r0 := byName["eth0"]
	if !r0.GotOffer {
		t.Error("eth0: GotOffer should be true")
	}
	if r0.OfferServer != "192.168.1.1" {
		t.Errorf("eth0: OfferServer = %q, want 192.168.1.1", r0.OfferServer)
	}

	r1 := byName["eth1"]
	if r1.GotOffer {
		t.Error("eth1: GotOffer should be false")
	}
	if r1.OfferServer != "" {
		t.Errorf("eth1: OfferServer should be empty, got %q", r1.OfferServer)
	}
}

// TestFakeProber_MissingEntry verifies that an interface not present in
// FakeProber.Results gets a default result (GotOffer=false).
func TestFakeProber_MissingEntry(t *testing.T) {
	prober := &detect.FakeProber{
		Results: map[string]detect.ProbeResult{}, // empty
	}

	results, err := prober.Probe(context.Background(), []string{"eth0"})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].GotOffer {
		t.Error("missing entry should default to GotOffer=false")
	}
}

// TestFakeProber_Error verifies that setting FakeProber.Err propagates to the
// caller without crashing.
func TestFakeProber_Error(t *testing.T) {
	sentinel := errors.New("probe error")
	prober := &detect.FakeProber{Err: sentinel}

	_, err := prober.Probe(context.Background(), []string{"eth0"})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// TestFakeProber_EmptyInput verifies that probing an empty interface list
// returns an empty result slice without error.
func TestFakeProber_EmptyInput(t *testing.T) {
	prober := &detect.FakeProber{}
	results, err := prober.Probe(context.Background(), nil)
	if err != nil {
		t.Fatalf("Probe(nil): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
