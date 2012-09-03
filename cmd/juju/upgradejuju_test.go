package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type UpgradeJujuSuite struct {
	repoSuite
}

var _ = Suite(&UpgradeJujuSuite{})

func (s *UpgradeJujuSuite) TestVanillaUpgrade(c *C) {
	m0, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)



test with already uploaded tools in various configurations.

test with uploading, and with bump version
