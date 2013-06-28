// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineagent_test

import (
	"fmt"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type suite struct {
	testing.JujuConnSuite
	st      *api.State
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
	c.Assert(m.Life(), Equals, params.Alive)
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
	c.Assert(m.Life(), Equals, params.Alive)

	err = s.machine.Destroy()
	c.Assert(err, IsNil)

	err = m.Refresh()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, params.Dying)

	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)

	err = m.Refresh()
	c.Assert(err, ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
}

func (s *suite) TestMachineSetPassword(c *C) {
	m, err := s.st.MachineAgent().Machine(s.machine.Id())
	c.Assert(err, IsNil)

	err = m.SetPassword("foo")
	c.Assert(err, IsNil)

	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine.PasswordValid("bar"), Equals, false)
	c.Assert(s.machine.PasswordValid("foo"), Equals, true)

	// Check that we cannot log in to mongo with the wrong password.
	info := s.StateInfo(c)
	info.Tag = m.Tag()
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(err, checkers.Satisfies, errors.IsUnauthorizedError)

	// Check that we can log in with the correct password
	info.Password = "foo"
	st, err := state.Open(info, state.DialOpts{})
	c.Assert(err, IsNil)
	st.Close()
}

func tryOpenState(info *state.Info) error {
	st, err := state.Open(info, state.DialOpts{})
	if err == nil {
		st.Close()
	}
	return err
}
