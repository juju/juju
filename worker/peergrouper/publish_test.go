// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/peergrouper"
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
	statePublish := peergrouper.NewPublisher(&mock, false)

	hostPorts1 := network.AddressesWithPort(network.NewAddresses("testing1.invalid", "127.0.0.1"), 1234)
	hostPorts2 := network.AddressesWithPort(network.NewAddresses("testing2.invalid", "127.0.0.2"), 1234)

	// statePublish.publishAPIServers should not update state a second time.
	apiServers := [][]network.HostPort{hostPorts1}
	for i := 0; i < 2; i++ {
		err := statePublish.PublishAPIServers(apiServers, nil)
		c.Assert(err, gc.IsNil)
	}

	c.Assert(mock.calls, gc.Equals, 1)
	c.Assert(mock.apiHostPorts, gc.DeepEquals, apiServers)

	apiServers = append(apiServers, hostPorts2)
	for i := 0; i < 2; i++ {
		err := statePublish.PublishAPIServers(apiServers, nil)
		c.Assert(err, gc.IsNil)
	}
	c.Assert(mock.calls, gc.Equals, 2)
	c.Assert(mock.apiHostPorts, gc.DeepEquals, apiServers)
}

func (s *publishSuite) TestPublisherSortsHostPorts(c *gc.C) {
	ipV4First := network.AddressesWithPort(network.NewAddresses("testing1.invalid", "127.0.0.1", "::1"), 1234)
	ipV6First := network.AddressesWithPort(network.NewAddresses("testing1.invalid", "::1", "127.0.0.1"), 1234)

	check := func(preferIPv6 bool, publish, expect []network.HostPort) {
		var mock mockAPIHostPortsSetter
		statePublish := peergrouper.NewPublisher(&mock, preferIPv6)
		for i := 0; i < 2; i++ {
			err := statePublish.PublishAPIServers([][]network.HostPort{publish}, nil)
			c.Assert(err, gc.IsNil)
		}
		c.Assert(mock.calls, gc.Equals, 1)
		c.Assert(mock.apiHostPorts, gc.DeepEquals, [][]network.HostPort{expect})
	}

	check(false, ipV6First, ipV4First)
	check(false, ipV4First, ipV4First)
	check(true, ipV4First, ipV6First)
	check(true, ipV6First, ipV6First)
}
