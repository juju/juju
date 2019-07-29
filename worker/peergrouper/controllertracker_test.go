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
	spaceName := network.SpaceName("ha-space")

	m := &controllerTracker{
		addresses: []network.Address{
			{
				Value:     "192.168.5.5",
				Scope:     network.ScopeCloudLocal,
				SpaceName: spaceName,
			},
			{
				Value:     "192.168.10.5",
				Scope:     network.ScopeCloudLocal,
				SpaceName: network.SpaceName("another-space"),
			},
			{
				Value: "localhost",
				Scope: network.ScopeMachineLocal,
			},
		},
	}

	addr, err := m.SelectMongoAddressFromSpace(666, spaceName)
	c.Assert(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.5.5:666")
}

func (s *machineTrackerSuite) TestSelectMongoAddressFromSpaceEmptyWhenNoAddressFound(c *gc.C) {
	m := &controllerTracker{
		id: "3",
		addresses: []network.Address{
			{
				Value: "localhost",
				Scope: network.ScopeMachineLocal,
			},
		},
	}

	addrs, err := m.SelectMongoAddressFromSpace(666, "bad-space")
	c.Check(addrs, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, `addresses for controller node "3" in space "bad-space" not found`)
}

func (s *machineTrackerSuite) TestSelectMongoAddressFromSpaceErrorForEmptySpace(c *gc.C) {
	m := &controllerTracker{
		id: "3",
	}

	_, err := m.SelectMongoAddressFromSpace(666, "")
	c.Check(err, gc.ErrorMatches, `empty space supplied as an argument for selecting Mongo address for controller node "3"`)
}

func (s *machineTrackerSuite) TestGetPotentialMongoHostPortsReturnsAllAddresses(c *gc.C) {
	m := &controllerTracker{
		id: "3",
		addresses: []network.Address{
			{
				Value: "192.168.5.5",
				Scope: network.ScopeCloudLocal,
			},
			{
				Value: "10.0.0.1",
				Scope: network.ScopeCloudLocal,
			},
			{
				Value: "185.159.16.82",
				Scope: network.ScopePublic,
			},
		},
	}

	addrs := network.HostPortsToStrings(m.GetPotentialMongoHostPorts(666))
	sort.Strings(addrs)
	c.Check(addrs, gc.DeepEquals, []string{"10.0.0.1:666", "185.159.16.82:666", "192.168.5.5:666"})
}
