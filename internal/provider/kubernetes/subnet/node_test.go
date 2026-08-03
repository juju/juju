// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"testing"

	"github.com/juju/tc"
)

type nodeSuite struct{}

func TestNode(t *testing.T) {
	tc.Run(t, &nodeSuite{})
}

func (s *nodeSuite) TestKubeRouterPluralAnnotation(c *tc.C) {
	clients := newClusterBuilder().addCore(
		node("a", nil, map[string]string{"kube-router.io/pod-cidrs": "10.20.0.0/24,fd20::/64"}),
	).clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.20.0.0/24", "fd20::/64"))
}

func (s *nodeSuite) TestKubeRouterLegacySingularAnnotation(c *tc.C) {
	clients := newClusterBuilder().addCore(
		node("a", nil, map[string]string{"kube-router.io/pod-cidr": "10.21.0.0/24"}),
	).clients()

	result, err := Subnets(c.Context(), clients)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, subnetInfo("10.21.0.0/24"))
}
