// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
)

type k8sNetworkingSuite struct {
}

func TestK8sNetworkingSuite(t *testing.T) {
	tc.Run(t, &k8sNetworkingSuite{})
}

func (s *k8sNetworkingSuite) TestSupportsSpaces(c *tc.C) {
	// Arrange
	envNet := &environNetworking{}

	// Act
	ok, err := envNet.SupportsSpaces()

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(ok, tc.IsFalse)
}

func (s *k8sNetworkingSuite) TestSubnets(c *tc.C) {
	// Arrange
	envNet := &environNetworking{}

	// Act
	result, err := envNet.Subnets(c.Context(), nil)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.DeepEquals, []network.SubnetInfo{
		{
			CIDR:       "0.0.0.0/0",
			ProviderId: "subnet-placeholder-0.0.0.0/0",
		}, {
			CIDR:       "::/0",
			ProviderId: "subnet-placeholder-::/0",
		},
	})
}
