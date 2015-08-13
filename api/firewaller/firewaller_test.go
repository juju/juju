// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

// NOTE: This suite is intended for embedding into other suites,
// so common code can be reused. Do not add test cases to it,
// otherwise they'll be run by each other suite that embeds it.
type firewallerSuite struct {
	testing.JujuConnSuite

	st       api.Connection
	machines []*state.Machine
	service  *state.Service
	charm    *state.Charm
	units    []*state.Unit

	firewaller *firewaller.State
}

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Reset previous machines and units (if any) and create 3
	// machines for the tests. The first one is a manager node.
	s.machines = make([]*state.Machine, 3)
	s.units = make([]*state.Unit, 3)

	var err error
	s.machines[0], err = s.State.AddMachine("quantal", state.JobManageEnviron, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[0].SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines[0].SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAsMachine(c, s.machines[0].Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)

	// Note that the specific machine ids allocated are assumed
	// to be numerically consecutive from zero.
	for i := 1; i <= 2; i++ {
		s.machines[i], err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Check(err, jc.ErrorIsNil)
	}
	// Create a service and three units for these machines.
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	// Add the rest of the units and assign them.
	for i := 0; i <= 2; i++ {
		s.units[i], err = s.service.AddUnit()
		c.Check(err, jc.ErrorIsNil)
		err = s.units[i].AssignToMachine(s.machines[i])
		c.Check(err, jc.ErrorIsNil)
	}

	// Create the firewaller API facade.
	s.firewaller = s.st.Firewaller()
	c.Assert(s.firewaller, gc.NotNil)
}
