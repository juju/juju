// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"errors"
	"testing"

	"github.com/juju/tc"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

type kubeovnSuite struct{}

func TestKubeOVN(t *testing.T) {
	tc.Run(t, &kubeovnSuite{})
}

func (s *kubeovnSuite) TestKubeOVNDefaultSubnet(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("join", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16"))
}

func (s *kubeovnSuite) TestKubeOVNExcludesJoinByCIDRAnnotation(c *tc.C) {
	// Join subnet uses a custom name but is identified via the node cidr
	// annotation. The node ovn.kubernetes.io/cidr value must not be consumed
	// as a pod CIDR.
	clients := newClusterBuilder().
		addCore(node("a", nil, map[string]string{
			"ovn.kubernetes.io/logical_switch": "transit",
			"ovn.kubernetes.io/cidr":           "100.64.0.0/16",
		})).
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("transit", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16"))
}

func (s *kubeovnSuite) TestKubeOVNNodeListErrorFallsBack(c *tc.C) {
	typed, apiext, dyn := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("transit", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		build()
	typed.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("boom")
	})
	clients := Clients{Typed: typed, Dynamic: dyn, APIExtensions: apiext}

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("0.0.0.0/0", "::/0"))
}

func (s *kubeovnSuite) TestKubeOVNIncludesDisableInterConnectionPodSubnet(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("custom", map[string]any{"cidrBlock": "10.17.0.0/16", "disableInterConnection": true}),
			kubeOVNSubnet("join", map[string]any{"cidrBlock": "100.64.0.0/16", "disableInterConnection": true}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16", "10.17.0.0/16"))
}

func (s *kubeovnSuite) TestKubeOVNIncludesPerNamespaceSubnet(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "default": true}),
			kubeOVNSubnet("ns-subnet", map[string]any{
				"cidrBlock":  "10.18.0.0/16",
				"namespaces": []any{"team-a"},
			}),
			kubeOVNSubnet("join", map[string]any{"cidrBlock": "100.64.0.0/16"}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16", "10.18.0.0/16"))
}

func (s *kubeovnSuite) TestKubeOVNDualStackSplit(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(kubeOVNSubnet("ovn-default", map[string]any{
			"cidrBlock": "10.16.0.0/16,fd00:10:16::/112",
			"protocol":  "Dual",
			"default":   true,
		})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16", "fd00:10:16::/112"))
}

func (s *kubeovnSuite) TestKubeOVNDefaultVPCPreferred(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("subnets", "kubeovn.io", "v1").
		addCR(
			kubeOVNSubnet("ovn-default", map[string]any{"cidrBlock": "10.16.0.0/16", "vpc": "ovn-cluster"}),
			kubeOVNSubnet("custom-vpc", map[string]any{"cidrBlock": "10.99.0.0/16", "vpc": "tenant"}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.16.0.0/16"))
}
