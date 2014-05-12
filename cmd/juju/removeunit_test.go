// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type RemoveUnitSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&RemoveUnitSuite{})

func runRemoveUnit(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveUnitCommand{}), args)
	return err
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *gc.C) {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	svc, _ := s.AssertService(c, "dummy", curl, 2, 0)

	err = runRemoveUnit(c, "dummy/0", "dummy/1", "dummy/2", "sillybilly/17")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "dummy/2" does not exist; unit "sillybilly/17" does not exist`)
	units, err := svc.AllUnits()
	c.Assert(err, gc.IsNil)
	for _, u := range units {
		c.Assert(u.Life(), gc.Equals, state.Dying)
	}
}
