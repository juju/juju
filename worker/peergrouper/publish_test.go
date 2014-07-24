// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type publishSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&publishSuite{})

type mockAPIHostPortsSetter struct {
	calls        int
	apiHostPorts [][]network.HostPort
}

func (s *mockAPIHostPortsSetter) SetAPIHostPorts(apiHostPorts [][]network.HostPort) error {
	s.calls++
	s.apiHostPorts = apiHostPorts
	return nil
}

func (s *publishSuite) TestPublisherSetsAPIHostPortsOnce(c *gc.C) {
	var mock mockAPIHostPortsSetter
	statePublish := newPublisher(&mock)

	hostPorts1 := network.AddressesWithPort(network.NewAddresses("testing1.invalid", "127.0.0.1"), 1234)
	hostPorts2 := network.AddressesWithPort(network.NewAddresses("testing2.invalid", "127.0.0.2"), 1234)

	// statePublish.publishAPIServers should not update state a second time.
	apiServers := [][]network.HostPort{hostPorts1}
	for i := 0; i < 2; i++ {
		err := statePublish.publishAPIServers(apiServers, nil)
		c.Assert(err, gc.IsNil)
	}

	c.Assert(mock.calls, gc.Equals, 1)
	c.Assert(mock.apiHostPorts, gc.DeepEquals, apiServers)

	apiServers = append(apiServers, hostPorts2)
	for i := 0; i < 2; i++ {
		err := statePublish.publishAPIServers(apiServers, nil)
		c.Assert(err, gc.IsNil)
	}
	c.Assert(mock.calls, gc.Equals, 2)
	c.Assert(mock.apiHostPorts, gc.DeepEquals, apiServers)
}
