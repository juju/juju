// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import "context"

// fallbackCIDRs is the catch-all pod CIDR set (all IPv4 + IPv6). It is returned
// when no CNI source can be confidently identified, and also directly by a
// discoverer that hits an RBAC denial (a present-but-unreadable CNI source) so
// that the resolver short-circuits to the safe fallback rather than degrading
// to a possibly-divergent node range.
var fallbackCIDRs = []string{"0.0.0.0/0", "::/0"}

// Discoverer recognises and reads the pod CIDRs for one IP-allocation strategy.
type Discoverer interface {
	// Name identifies the strategy (for logging/tracing).
	Name() string
	// Discover returns the pod CIDRs when this discoverer positively identifies
	// its source, or an empty slice when its source is absent / not applicable
	// (so the resolver moves on). An error is returned only for genuinely
	// unexpected failures; the resolver logs it and continues. Discovery never
	// fails the caller.
	Discover(ctx context.Context, clients Clients) ([]string, error)
}

// Resolve walks the chain and returns the CIDRs from the first discoverer that
// yields a confident, non-empty result; if every discoverer is empty it returns
// nil. A discoverer that hits an RBAC denial returns the fallback CIDRs, which
// (being non-empty) short-circuits the chain here.
func Resolve(ctx context.Context, clients Clients, chain ...Discoverer) []string {
	for _, d := range chain {
		cidrs, err := d.Discover(ctx, clients)
		if err != nil {
			logger.Infof(ctx, "pod-subnet discoverer %q failed, skipping: %v", d.Name(), err)
			continue
		}
		if len(cidrs) > 0 {
			logger.Debugf(ctx, "pod-subnet discoverer %q matched CIDRs %v", d.Name(), cidrs)
			return cidrs
		}
	}
	return nil
}

// chain returns the ordered list of discoverers. The ordering is critical:
// CNI-specific discoverers run first (each decides authority for its own mode),
// and the generic node-pod-cidr fallback runs last.
func chain() []Discoverer {
	return []Discoverer{
		calicoDiscoverer{},
		ciliumDiscoverer{},
		ovnKubernetesDiscoverer{},
		kubeOVNDiscoverer{},
		nodePodCIDRDiscoverer{},
	}
}
