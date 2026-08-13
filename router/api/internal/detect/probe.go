package detect

import (
	"context"
)

// ProbeResult holds the DHCP discovery outcome for one network interface.
type ProbeResult struct {
	Interface   string
	GotOffer    bool
	OfferServer string // DHCP server IP from option 54; empty when no offer
}

// Prober sends DHCP DISCOVER packets and reports which interfaces received
// an OFFER. The interface is designed to be replaced in tests with a
// FakeProber that returns pre-configured results without touching the network.
type Prober interface {
	Probe(ctx context.Context, ifaces []string) ([]ProbeResult, error)
}

// FakeProber is a test double that returns pre-configured results without
// touching the network. Set Results before calling Probe.
type FakeProber struct {
	// Results maps interface name → ProbeResult. Missing entries default to
	// GotOffer=false.
	Results map[string]ProbeResult
	// Err, if non-nil, is returned from every Probe call (simulates hard failure).
	Err error
}

// Probe returns the pre-configured results from f.Results.
func (f *FakeProber) Probe(_ context.Context, ifaces []string) ([]ProbeResult, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]ProbeResult, len(ifaces))
	for i, name := range ifaces {
		if r, ok := f.Results[name]; ok {
			out[i] = r
		} else {
			out[i] = ProbeResult{Interface: name}
		}
	}
	return out, nil
}

// compile-time interface check for FakeProber
var _ Prober = &FakeProber{}
