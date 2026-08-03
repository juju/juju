// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

type k8sNetworkingSuite struct{}

func TestK8sNetworkingSuite(t *testing.T) {
	tc.Run(t, &k8sNetworkingSuite{})
}

func (s *k8sNetworkingSuite) TestSupportsSpaces(c *tc.C) {
	envNet := &environNetworking{}

	ok, err := envNet.SupportsSpaces()

	c.Assert(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsFalse)
}

func (s *k8sNetworkingSuite) TestSubnetsNilClientFallsBack(c *tc.C) {
	envNet := &environNetworking{}

	result, err := envNet.Subnets(c.Context(), nil)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, network.FallbackSubnetInfo)
}
