// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
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
	statePublish := &CachingAPIHostPortsSetter{APIHostPortsSetter: &mock}

	hostPorts1 := network.NewHostPorts(1234, "testing1.invalid", "127.0.0.1")
	hostPorts2 := network.NewHostPorts(1234, "testing2.invalid", "127.0.0.2")

	// statePublish.SetAPIHostPorts should not update state a second time.
	apiServers := [][]network.HostPort{hostPorts1}
	for i := 0; i < 2; i++ {
		err := statePublish.SetAPIHostPorts(apiServers)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Assert(mock.calls, gc.Equals, 1)
	c.Assert(mock.apiHostPorts, gc.DeepEquals, apiServers)

	apiServers = append(apiServers, hostPorts2)
	for i := 0; i < 2; i++ {
		err := statePublish.SetAPIHostPorts(apiServers)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Assert(mock.calls, gc.Equals, 2)
	c.Assert(mock.apiHostPorts, gc.DeepEquals, apiServers)
}

func (s *publishSuite) TestPublisherSortsHostPorts(c *gc.C) {
	ipV4First := network.NewHostPorts(1234, "testing1.invalid", "127.0.0.1", "::1")
	ipV6First := network.NewHostPorts(1234, "testing1.invalid", "::1", "127.0.0.1")

	check := func(publish, expect []network.HostPort) {
		var mock mockAPIHostPortsSetter
		statePublish := &CachingAPIHostPortsSetter{APIHostPortsSetter: &mock}
		for i := 0; i < 2; i++ {
			err := statePublish.SetAPIHostPorts([][]network.HostPort{publish})
			c.Assert(err, jc.ErrorIsNil)
		}
		c.Assert(mock.calls, gc.Equals, 1)
		c.Assert(mock.apiHostPorts, gc.DeepEquals, [][]network.HostPort{expect})
	}

	check(ipV6First, ipV4First)
	check(ipV4First, ipV4First)
}

func (s *publishSuite) TestPublisherRejectsNoServers(c *gc.C) {
	var mock mockAPIHostPortsSetter
	statePublish := &CachingAPIHostPortsSetter{APIHostPortsSetter: &mock}
	err := statePublish.SetAPIHostPorts(nil)
	c.Assert(err, gc.ErrorMatches, "no API servers specified")
}
