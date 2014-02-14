// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/charmrevisionupdater"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater/testing"
	"launchpad.net/juju-core/utils"
)

type versionUpdaterSuite struct {
	testing.CharmSuite

	updater *charmrevisionupdater.State
}

var _ = gc.Suite(&versionUpdaterSuite{})

func (s *versionUpdaterSuite) SetUpTest(c *gc.C) {
	s.CharmSuite.SetUpTest(c)

	machine, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	st := s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")
	c.Assert(st, gc.NotNil)

	s.updater = charmrevisionupdater.NewState(st)
	c.Assert(s.updater, gc.NotNil)
}

func (s *versionUpdaterSuite) TestUpdateRevisions(c *gc.C) {
	s.SetupScenario(c)
	err := s.updater.UpdateLatestRevisions()
	c.Assert(err, gc.IsNil)

	curl := charm.MustParseURL("cs:quantal/mysql")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, gc.IsNil)
	c.Assert(pending.String(), gc.Equals, "cs:quantal/mysql-23")
}
