// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/network"
)

type filteringUnitTests struct {
}

var _ = gc.Suite(&filteringUnitTests{})

func (f *filteringUnitTests) TestMatchPorts(c *gc.C) {

	match, ok, err := client.MatchPorts([]string{"80/tcp"}, network.Port{"tcp", 80})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchPorts([]string{"90/tcp"}, network.Port{"tcp", 80})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsFalse)
}

func (s *filteringUnitTests) TestMatchSubnet(c *gc.C) {

	match, ok, err := client.MatchSubnet([]string{"localhost"}, "127.0.0.1")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchSubnet([]string{"127.0.0.1"}, "127.0.0.1")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchSubnet([]string{"localhost"}, "10.0.0.1")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsFalse)
}
