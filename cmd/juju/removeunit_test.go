package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type RemoveUnitSuite struct {
	repoSuite
}

var _ = Suite(&RemoveUnitSuite{})

func runRemoveUnit(c *C, args ...string) error {
	com := &RemoveUnitCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "series", "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy1")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "dummy1", curl, 2, 0)

	err = runRemoveUnit(c, "dummy1/0", "dummy1/1")
	c.Assert(err, IsNil)

	// can't remove a nonexistent unit.
	err = runRemoveUnit(c, "dummy1/5")
	c.Assert(err, ErrorMatches, "unit \"dummy1/5\" not found")

	// Removing a unit that is dying is not an error.
	err = runRemoveUnit(c, "dummy1/1")
	c.Assert(err, IsNil)
}
