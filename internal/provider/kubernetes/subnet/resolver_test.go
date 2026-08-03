// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/tc"
)

type resolverSuite struct{}

func TestResolver(t *testing.T) {
	tc.Run(t, &resolverSuite{})
}

// stubDiscoverer is a test harness for exercising the Resolve engine directly.
type stubDiscoverer struct {
	name  string
	cidrs []string
	err   error
}

func (d stubDiscoverer) Name() string { return d.name }
func (d stubDiscoverer) Discover(context.Context, Clients) ([]string, error) {
	return d.cidrs, d.err
}

func (s *resolverSuite) TestResolveSkipsErroringDiscoverer(c *tc.C) {
	clients := Clients{}
	cidrs := Resolve(c.Context(), clients,
		stubDiscoverer{name: "boom", err: errors.New("boom")},
		stubDiscoverer{name: "match", cidrs: []string{"10.0.0.0/24"}},
	)
	c.Check(cidrs, tc.DeepEquals, []string{"10.0.0.0/24"})
}

func (s *resolverSuite) TestResolveStopsAtFirstNonEmpty(c *tc.C) {
	clients := Clients{}
	cidrs := Resolve(c.Context(), clients,
		stubDiscoverer{name: "first", cidrs: []string{"10.0.0.0/24"}},
		stubDiscoverer{name: "second", cidrs: []string{"10.1.0.0/24"}},
	)
	c.Check(cidrs, tc.DeepEquals, []string{"10.0.0.0/24"})
}

func (s *resolverSuite) TestChainFirstMatchShortCircuits(c *tc.C) {
	// Calico matches; Kube-OVN Subnets also exist but must not be consulted.
	clients := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil),
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *resolverSuite) TestChainAllEmptyFallsBack(c *tc.C) {
	clients := newClusterBuilder().clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}
