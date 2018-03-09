// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

type machineTrackerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&machineTrackerSuite{})

func (s *machineTrackerSuite) TestSelectMongoAddressReturnsCorrectAddressWithSpace(c *gc.C) {
	spaceName := network.SpaceName("ha-space")

	m := &machineTracker{
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

	addr, err := m.SelectMongoAddress(666, spaceName)
	c.Assert(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.5.5:666")
}

func (s *machineTrackerSuite) TestSelectMongoAddressReturnsCorrectAddressWithoutSpace(c *gc.C) {
	spaceName := network.SpaceName("")

	m := &machineTracker{
		addresses: []network.Address{
			{
				Value: "192.168.5.5",
				Scope: network.ScopeCloudLocal,
			},
			{
				Value: "localhost",
				Scope: network.ScopeMachineLocal,
			},
		},
	}

	addr, err := m.SelectMongoAddress(666, spaceName)
	c.Assert(err, gc.IsNil)
	c.Check(addr, gc.Equals, "192.168.5.5:666")
}

func (s *machineTrackerSuite) TestSelectMongoAddressReturnsErrNoSpaceMultipleCloudLocal(c *gc.C) {
	spaceName := network.SpaceName("")

	m := &machineTracker{
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
				Value: "localhost",
				Scope: network.ScopeMachineLocal,
			},
		},
	}

	addr, err := m.SelectMongoAddress(666, spaceName)
	c.Check(err, gc.ErrorMatches, `machine "3" has more than one non-local address and juju-ha-space is not set`)
	c.Check(addr, gc.Equals, "")
}

func (s *machineTrackerSuite) TestSelectMongoAddressReturnsEmptyWhenNoAddressFound(c *gc.C) {
	spaceName := network.SpaceName("")

	m := &machineTracker{
		id: "3",
		addresses: []network.Address{
			{
				Value: "localhost",
				Scope: network.ScopeMachineLocal,
			},
		},
	}

	addr, err := m.SelectMongoAddress(666, spaceName)
	c.Assert(err, gc.IsNil)
	c.Check(addr, gc.Equals, "")
}
