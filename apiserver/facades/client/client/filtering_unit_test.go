// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/core/network"
)

type filteringUnitTests struct {
}

var _ = gc.Suite(&filteringUnitTests{})

func (f *filteringUnitTests) TestMatchPortRanges(c *gc.C) {

	match, ok, err := client.MatchPortRanges([]string{"80/tcp"}, network.PortRange{FromPort: 80, ToPort: 80, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchPortRanges([]string{"80-90/tcp"}, network.PortRange{FromPort: 80, ToPort: 90, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchPortRanges([]string{"90/tcp"}, network.PortRange{FromPort: 80, ToPort: 90, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchPortRanges([]string{"70"}, network.PortRange{FromPort: 7070, ToPort: 7070, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070"}, network.PortRange{FromPort: 7070, ToPort: 7070, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070/udp"}, network.PortRange{FromPort: 7070, ToPort: 7070, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070/tcp"}, network.PortRange{FromPort: 7065, ToPort: 7069, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsFalse)

	match, ok, err = client.MatchPortRanges([]string{"7070/tcp"}, network.PortRange{FromPort: 7069, ToPort: 7071, Protocol: "tcp"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)
}

func (s *filteringUnitTests) TestMatchSubnet(c *gc.C) {

	// We do not resolve hostnames.
	match, ok, err := client.MatchSubnet([]string{"localhost"}, "127.0.0.1")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsFalse)
	c.Check(match, jc.IsFalse)

	match, ok, err = client.MatchSubnet([]string{"127.0.0.1"}, "127.0.0.1")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)

	match, ok, err = client.MatchSubnet([]string{"localhost"}, "10.0.0.1")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsFalse)
	c.Check(match, jc.IsFalse)

	match, ok, err = client.MatchSubnet([]string{"testing.local"}, "testing.local")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ok, jc.IsTrue)
	c.Check(match, jc.IsTrue)
}
