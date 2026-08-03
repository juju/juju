// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"testing"

	"github.com/juju/tc"
)

type ciliumSuite struct{}

func TestCilium(t *testing.T) {
	tc.Run(t, &ciliumSuite{})
}

func (s *ciliumSuite) TestCiliumClusterPoolConfigMap(c *tc.C) {
	clients := newClusterBuilder().
		addCore(
			node("a", []string{"10.244.0.0/24"}, nil), // divergent node spec, must be ignored
			ciliumConfig(map[string]string{
				"ipam":                   "cluster-pool",
				"cluster-pool-ipv4-cidr": "10.0.0.0/8",
				"cluster-pool-ipv6-cidr": "fd00::/48",
			}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.0.0.0/8", "fd00::/48"))
}

func (s *ciliumSuite) TestCiliumClusterPoolDefaultModeFromConfigMap(c *tc.C) {
	// ipam key absent => default cluster-pool.
	clients := newClusterBuilder().
		addCore(ciliumConfig(map[string]string{
			"cluster-pool-ipv4-cidr": "10.0.0.0/8",
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.0.0.0/8"))
}

func (s *ciliumSuite) TestCiliumClusterPoolFromCiliumNode(c *tc.C) {
	clients := newClusterBuilder().
		addCore(ciliumConfig(map[string]string{"ipam": "cluster-pool"})).
		addCRD("ciliumnodes", "cilium.io", "v2").
		addCR(cr("cilium.io/v2", "CiliumNode", "node-a", map[string]any{
			"ipam": map[string]any{"podCIDRs": []any{"10.1.0.0/24"}},
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.1.0.0/24"))
}

func (s *ciliumSuite) TestCiliumMultiPoolFromCRD(c *tc.C) {
	clients := newClusterBuilder().
		addCore(ciliumConfig(map[string]string{"ipam": "multi-pool"})).
		addCRD("ciliumpodippools", "cilium.io", "v2alpha1").
		addCR(cr("cilium.io/v2alpha1", "CiliumPodIPPool", "default", map[string]any{
			"ipv4": map[string]any{"cidrs": []any{"10.10.0.0/16"}},
			"ipv6": map[string]any{"cidrs": []any{"fd00::/48"}},
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.10.0.0/16", "fd00::/48"))
}

func (s *ciliumSuite) TestCiliumKubernetesModeUsesNodeSpec(c *tc.C) {
	clients := newClusterBuilder().
		addCore(
			node("a", []string{"10.244.0.0/24"}, nil),
			ciliumConfig(map[string]string{"ipam": "kubernetes"}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.244.0.0/24"))
}

func (s *ciliumSuite) TestCiliumVPCNativeModesYieldFallback(c *tc.C) {
	for _, mode := range []string{"eni", "azure", "alibabacloud", "crd"} {
		clients := newClusterBuilder().
			addCore(
				node("a", []string{"10.244.0.0/24"}, nil),
				ciliumConfig(map[string]string{"ipam": mode}),
			).
			clients()

		result, err := Subnets(c.Context(), clients)

		c.Assert(err, tc.ErrorIsNil, tc.Commentf("mode %q", mode))
		c.Check(result, tc.DeepEquals, subnetInfo("0.0.0.0/0", "::/0"), tc.Commentf("mode %q", mode))
	}
}

func (s *ciliumSuite) TestCiliumAnnotationNotConsumed(c *tc.C) {
	// No cilium-config, no CRDs, no node spec: the cilium.io/*-pod-cidr
	// annotation must not be consumed; result is the fallback.
	clients := newClusterBuilder().
		addCore(node("a", nil, map[string]string{"cilium.io/ipv4-pod-cidr": "10.5.0.0/16"})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}
