// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"sort"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/testing"
)

type machineTrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&machineTrackerSuite{})

func (s *machineTrackerSuite) TestGetPotentialMongoHostPortsReturnsAllAddresses(c *gc.C) {
	m := &controllerTracker{
		id: "3",
		addresses: []network.SpaceAddress{
			network.NewSpaceAddress("192.168.5.5", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("185.159.16.82", network.WithScope(network.ScopePublic)),
		},
	}

	addrs := m.GetPotentialMongoHostPorts(666).HostPorts().Strings()
	sort.Strings(addrs)
	c.Check(addrs, gc.DeepEquals, []string{"10.0.0.1:666", "185.159.16.82:666", "192.168.5.5:666"})
}
