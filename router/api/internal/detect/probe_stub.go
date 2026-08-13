//go:build !linux

package detect

import "context"

// RealProber is a stub on non-Linux platforms. It always returns empty results
// since raw DHCP sockets require Linux kernel interfaces (AF_PACKET).
// Use FakeProber in tests.
type RealProber struct{}

// NewRealProber returns a stub Prober on non-Linux platforms.
func NewRealProber() Prober {
	return &RealProber{}
}

// Probe always returns empty (no offer) results on non-Linux.
func (p *RealProber) Probe(_ context.Context, ifaces []string) ([]ProbeResult, error) {
	results := make([]ProbeResult, len(ifaces))
	for i, name := range ifaces {
		results[i].Interface = name
	}
	return results, nil
}

// compile-time interface check
var _ Prober = &RealProber{}
