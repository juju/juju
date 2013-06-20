// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineagent_test

import (
	"fmt"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type suite struct {
	testing.JujuConnSuite
	st *api.State
	machine *state.Machine
}

var _ = Suite(&suite{})

func (s *suite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	// Create a machine so we can log in as its agent.
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machine.SetPassword("password")
	c.Assert(err, IsNil)
	s.st = s.OpenAPIAs(c, s.machine.Tag(), "password")
}

func (s *suite) TearDownTest(c *C) {
	err := s.st.Close()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *suite) TestMachine(c *C) {
	m, err := s.st.MachineAgent().Machine("42")
	c.Assert(err, ErrorMatches, "permission denied")
	c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)
	c.Assert(m, IsNil)

	m, err = s.st.MachineAgent().Machine(s.machine.Id())
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, s.machine.Id())
	c.Assert(m.Life(), Equals, params.Life("alive"))
	c.Assert(m.Jobs(), DeepEquals, []params.MachineJob{params.JobHostUnits})

	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)

	m, err = s.st.MachineAgent().Machine(s.machine.Id())
	c.Assert(err, ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
	c.Assert(m, IsNil)
}

func (s *suite) TestMachineRefresh(c *C) {
	m, err := s.st.MachineAgent().Machine(s.machine.Id())
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, params.Life("alive"))

	err = s.machine.Destroy()
	c.Assert(err, IsNil)

	err = m.Refresh()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, params.Life("dying"))

	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)

	err = m.Refresh()
	c.Assert(err, ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
}