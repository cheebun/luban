//go:build linux

package detect

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4/nclient4"
)

// RealProber is the production DHCP prober for Linux. It uses AF_PACKET raw
// sockets via nclient4, which requires root privileges.
//
// Design note: unconfigured mode enslaves all ethernet ports to br0.
// nclient4 uses AF_PACKET which bypasses the bridge and sends directly on the
// port's L2 channel, so it works correctly on bridge-member interfaces — the
// DISCOVER exits the enslaved port, and the OFFER arrives back on the same
// port regardless of bridge membership. If this proves unreliable in practice,
// the fallback is to bind to br0 and correlate received offers by source MAC
// (matching the interface's hardware address written into the DISCOVER XID),
// but AF_PACKET on bridge members has been reliable in testing.
type RealProber struct{}

// NewRealProber returns a Prober that sends real DHCP DISCOVER packets.
// Only available on Linux; use FakeProber in non-Linux environments or tests.
func NewRealProber() Prober {
	return &RealProber{}
}

// Probe probes each interface in parallel with a per-port 4-second timeout,
// further capped by the caller's context deadline.
func (p *RealProber) Probe(ctx context.Context, ifaces []string) ([]ProbeResult, error) {
	results := make([]ProbeResult, len(ifaces))
	for i, name := range ifaces {
		results[i].Interface = name
	}

	var wg sync.WaitGroup
	for i, name := range ifaces {
		wg.Add(1)
		go func(idx int, ifName string) {
			defer wg.Done()
			pCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			probeOne(pCtx, ifName, &results[idx])
		}(i, name)
	}
	wg.Wait()
	return results, nil
}

// probeOne sends a single DHCP DISCOVER on ifName and updates r with the
// result. Errors (timeout, no offer, permission denied) leave GotOffer=false.
func probeOne(ctx context.Context, ifName string, r *ProbeResult) {
	c, err := nclient4.New(ifName, nclient4.WithRetry(1))
	if err != nil {
		return
	}
	defer func() { _ = c.Close() }()

	offer, err := c.DiscoverOffer(ctx)
	if err != nil {
		return
	}

	r.GotOffer = true

	// ServerIdentifier() returns option 54 (the canonical DHCP server address).
	// Fall back to the siaddr header field if option 54 is absent.
	if sid := offer.ServerIdentifier(); len(sid) > 0 {
		r.OfferServer = sid.String()
	} else if !offer.ServerIPAddr.Equal(net.IPv4zero) {
		r.OfferServer = offer.ServerIPAddr.String()
	}
}

// compile-time interface check
var _ Prober = &RealProber{}
