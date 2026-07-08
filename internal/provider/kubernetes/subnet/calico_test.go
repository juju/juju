// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"testing"

	"github.com/juju/tc"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

type calicoSuite struct{}

func TestCalico(t *testing.T) {
	tc.Run(t, &calicoSuite{})
}

func (s *calicoSuite) TestCalicoIPPoolV1(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil)).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *calicoSuite) TestCalicoIPPoolV3(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("ippools", "projectcalico.org", "v3").
		addCR(calicoIPPool("projectcalico.org/v3", "default-ipv4", "192.168.0.0/16", nil)).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *calicoSuite) TestCalicoIPPoolDualStack(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil),
			calicoIPPool("crd.projectcalico.org/v1", "default-ipv6", "fd00::/48", nil),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16", "fd00::/48"))
}

func (s *calicoSuite) TestCalicoIPPoolExcludesDisabled(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "enabled", "192.168.0.0/16", nil),
			calicoIPPool("crd.projectcalico.org/v1", "disabled", "10.99.0.0/16", map[string]any{"disabled": true}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *calicoSuite) TestCalicoIPPoolExcludesNonWorkloadAllowedUses(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(
			calicoIPPool("crd.projectcalico.org/v1", "workload", "192.168.0.0/16",
				map[string]any{"allowedUses": []any{"Workload"}}),
			calicoIPPool("crd.projectcalico.org/v1", "tunnel-only", "10.99.0.0/16",
				map[string]any{"allowedUses": []any{"Tunnel"}}),
		).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *calicoSuite) TestCalicoIPPoolIncludesAssignmentModeManual(c *tc.C) {
	clients := newClusterBuilder().
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(calicoIPPool("crd.projectcalico.org/v1", "manual", "192.168.0.0/16",
			map[string]any{"assignmentMode": "Manual"})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *calicoSuite) TestCalicoIPPoolIgnoresDivergentNodeSpec(c *tc.C) {
	// calico-ipam: a populated but divergent node.Spec.PodCIDR must be ignored.
	clients := newClusterBuilder().
		addCore(node("a", []string{"10.244.0.0/24"}, nil)).
		addCRD("ippools", "crd.projectcalico.org", "v1").
		addCR(calicoIPPool("crd.projectcalico.org/v1", "default-ipv4", "192.168.0.0/16", nil)).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("192.168.0.0/16"))
}

func (s *calicoSuite) TestCalicoHostLocalUsesNodeSpec(c *tc.C) {
	// IPPool CRD installed but zero pools => host-local/Canal => node spec.
	clients := newClusterBuilder().
		addCore(node("a", []string{"10.244.0.0/24"}, nil)).
		addCRD("ippools", "crd.projectcalico.org", "v1").
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.244.0.0/24"))
}

func (s *calicoSuite) TestCalicoHostLocalWithCanalConfig(c *tc.C) {
	clients := newClusterBuilder().
		addCore(
			node("a", []string{"10.244.1.0/24"}, nil),
			configMap("kube-system", "canal-config", map[string]string{
				"net-conf.json": `{"Network":"10.244.0.0/16","Backend":{"Type":"vxlan"}}`,
			}),
		).
		addCRD("ippools", "crd.projectcalico.org", "v1").
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.244.0.0/16", "10.244.1.0/24"))
}

func (s *calicoSuite) TestCalicoCRDAbsentContributesNothing(c *tc.C) {
	// IPPool CRD absent (distinct from empty pools): a node with the calico
	// host-IP annotation must NOT be consumed; result falls through to the
	// fallback (no node spec, no other source).
	clients := newClusterBuilder().
		addCore(node("a", nil, map[string]string{"projectcalico.org/IPv4Address": "10.0.0.5/24"})).
		clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}

func (s *calicoSuite) TestCalicoListForbiddenWithPopulatedNodeYieldsFallback(c *tc.C) {
	b := newClusterBuilder().
		addCore(node("a", []string{"10.244.0.0/24"}, nil)).
		addCRD("ippools", "crd.projectcalico.org", "v1")
	typed, apiext, dyn := b.build()
	dyn.PrependReactor("list", "ippools", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, forbidden("ippools")
	})
	clients := Clients{Typed: typed, Dynamic: dyn, APIExtensions: apiext}

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("0.0.0.0/0", "::/0"))
}
