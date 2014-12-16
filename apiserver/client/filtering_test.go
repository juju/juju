// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/network"
)

type filteringUnitTests struct {
}

var _ = gc.Suite(&filteringUnitTests{})

func (f *filteringUnitTests) TestMatchPortRanges(c *gc.C) {

	match, ok, err := client.MatchPortRanges([]string{"80/tcp"}, network.PortRange{80, 80, "tcp"})
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, true)

	match, ok, err = client.MatchPortRanges([]string{"80-90/tcp"}, network.PortRange{80, 90, "tcp"})
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, true)

	match, ok, err = client.MatchPortRanges([]string{"90/tcp"}, network.PortRange{80, 90, "tcp"})
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, false)
}

func (s *filteringUnitTests) TestMatchSubnet(c *gc.C) {

	match, ok, err := client.MatchSubnet([]string{"localhost"}, "127.0.0.1")
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, true)

	match, ok, err = client.MatchSubnet([]string{"127.0.0.1"}, "127.0.0.1")
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, true)

	match, ok, err = client.MatchSubnet([]string{"localhost"}, "10.0.0.1")
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, false)
}
