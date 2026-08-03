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

type discoverySuite struct{}

func TestDiscovery(t *testing.T) {
	tc.Run(t, &discoverySuite{})
}

func (s *discoverySuite) TestSubnetsNodePodCIDRs(c *tc.C) {
	clients := newClusterBuilder().addCore(
		node("a", []string{"10.10.0.0/24"}, nil),
		node("b", []string{"fd10::/64", "10.10.1.0/24"}, nil),
		node("c", []string{"10.10.0.0/24", "not-a-cidr"}, nil),
	).clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.10.0.0/24", "10.10.1.0/24", "fd10::/64"))
}

func (s *discoverySuite) TestSubnetsNodeListErrorFallsBack(c *tc.C) {
	typed, apiext, dyn := newClusterBuilder().build()
	typed.PrependReactor("list", "nodes", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("boom")
	})
	clients := Clients{Typed: typed, Dynamic: dyn, APIExtensions: apiext}

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, fallbackResult)
}
