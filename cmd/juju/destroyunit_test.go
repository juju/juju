package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type DestroyUnitSuite struct {
	repoSuite
}

var _ = Suite(&DestroyUnitSuite{})

func runDestroyUnit(c *C, args ...string) error {
	return testing.RunCommand(c, &DestroyUnitCommand{}, args)
}

func (s *DestroyUnitSuite) TestDestroyUnit(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	svc, _ := s.assertService(c, "dummy", curl, 2, 0)

	err = runDestroyUnit(c, "dummy/0", "dummy/1", "dummy/2", "sillybilly/17")
	c.Assert(err, ErrorMatches, `some units were not destroyed: unit "dummy/2" does not exist; unit "sillybilly/17" does not exist`)
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	for _, u := range units {
		c.Assert(u.Life(), Equals, state.Dying)
	}
}
