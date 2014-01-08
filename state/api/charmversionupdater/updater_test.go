// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionupdater_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/charmversionupdater"
	"launchpad.net/juju-core/state/apiserver/charmversionupdater/testing"
	"launchpad.net/juju-core/utils"
)

type versionUpdaterSuite struct {
	testing.CharmSuite

	updater *charmversionupdater.State
}

var _ = gc.Suite(&versionUpdaterSuite{})

func (s *versionUpdaterSuite) SetUpTest(c *gc.C) {
	s.CharmSuite.SetUpTest(c)

	machine, err := s.State.AddMachine("quantal", state.JobManageState)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	st := s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")
	c.Assert(st, gc.NotNil)

	s.updater = charmversionupdater.NewState(st)
	c.Assert(s.updater, gc.NotNil)
}

func (s *versionUpdaterSuite) TestUpdateVersions(c *gc.C) {
	s.SetupScenario(c)
	err := s.updater.UpdateVersions()
	c.Assert(err, gc.IsNil)

	svc, err := s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "")
	svc, err = s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "out of date (available: 23)")
	u, err := s.State.Unit("mysql/0")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "unknown")
	u, err = s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "")
	u, err = s.State.Unit("wordpress/1")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "unknown")

}
