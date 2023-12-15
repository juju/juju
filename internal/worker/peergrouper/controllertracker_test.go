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

func (s *machineTrackerSuite) TestSelectMongoAddressFromSpaceReturnsCorrectAddress(c *gc.C) {
	space := network.SpaceInfo{
		ID:   "123",
		Name: network.SpaceName("ha-space"),
	}

	m := &controllerTracker{
		addresses: []network.SpaceAddress{
			network.NewSpaceAddress("192.168.5.5", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("192.168.10.5", network.WithScope(network.ScopeCloudLocal)),
			network.NewSpaceAddress("localhost", network.WithScope(network.ScopeMachineLocal)),
		},
	}
	m.addresses[0].SpaceID = space.ID
	m.addresses[1].SpaceID = "456"

	addr, err := m.SelectMongoAddressFromSpace(666, space)
	c.Assert(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.5.5:666")
}

func (s *machineTrackerSuite) TestSelectMongoAddressFromSpaceEmptyWhenNoAddressFound(c *gc.C) {
	m := &controllerTracker{
		id: "3",
		addresses: []network.SpaceAddress{
			network.NewSpaceAddress("localhost", network.WithScope(network.ScopeMachineLocal))},
	}

	addrs, err := m.SelectMongoAddressFromSpace(666, network.SpaceInfo{ID: "whatever", Name: "bad-space"})
	c.Check(addrs, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, `addresses for controller node "3" in space "bad-space" not found`)
}

func (s *machineTrackerSuite) TestSelectMongoAddressFromSpaceErrorForEmptySpace(c *gc.C) {
	m := &controllerTracker{
		id: "3",
	}

	_, err := m.SelectMongoAddressFromSpace(666, network.SpaceInfo{})
	c.Check(err, gc.ErrorMatches, `empty space supplied as an argument for selecting Mongo address for controller node "3"`)
}

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
