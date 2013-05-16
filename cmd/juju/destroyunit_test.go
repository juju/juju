// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type DestroyUnitSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&DestroyUnitSuite{})

func runDestroyUnit(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &DestroyUnitCommand{}, args)
	return err
}

func (s *DestroyUnitSuite) TestDestroyUnit(c *C) {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	svc, _ := s.AssertService(c, "dummy", curl, 2, 0)

	err = runDestroyUnit(c, "dummy/0", "dummy/1", "dummy/2", "sillybilly/17")
	c.Assert(err, ErrorMatches, `some units were not destroyed: unit "dummy/2" does not exist; unit "sillybilly/17" does not exist`)
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	for _, u := range units {
		c.Assert(u.Life(), Equals, state.Dying)
	}
}
