// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/network"

	gc "gopkg.in/check.v1"
)

type filteringUnitTests struct {
}

var _ = gc.Suite(&filteringUnitTests{})

func (f *filteringUnitTests) TestMatchPorts(c *gc.C) {

	match, ok, err := client.MatchPorts([]string{"80/tcp"}, network.Port{"tcp", 80})
	c.Check(err, gc.IsNil)
	c.Check(ok, gc.Equals, true)
	c.Check(match, gc.Equals, true)

	match, ok, err = client.MatchPorts([]string{"90/tcp"}, network.Port{"tcp", 80})
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
