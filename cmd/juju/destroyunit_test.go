package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type DestroyUnitSuite struct {
	repoSuite
}

var _ = Suite(&DestroyUnitSuite{})

func runDestroyUnit(c *C, args ...string) error {
	com := &DestroyUnitCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *DestroyUnitSuite) TestDestroyUnit(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy1")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "dummy1", curl, 2, 0)

	err = runDestroyUnit(c, "dummy1/0", "dummy1/1")
	c.Assert(err, IsNil)
	err = runDestroyUnit(c, "dummy1/5")
	c.Assert(err, ErrorMatches, `cannot destroy units: unit "dummy1/5" is not alive`)
}
