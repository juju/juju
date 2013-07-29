// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

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
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, IsNil)
	err = s.machine.SetPassword("password")
	c.Assert(err, IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), "password", "fake_nonce")
}

func (s *suite) TearDownTest(c *C) {
	err := s.st.Close()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *suite) TestEntity(c *C) {
	m, err := s.st.Agent().Entity("42")
	c.Assert(err, ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), Equals, params.CodeUnauthorized)
	c.Assert(m, IsNil)

	m, err = s.st.Agent().Entity(s.machine.Tag())
	c.Assert(err, IsNil)
	c.Assert(m.Tag(), Equals, s.machine.Tag())
	c.Assert(m.Life(), Equals, params.Alive)
	c.Assert(m.Jobs(), DeepEquals, []params.MachineJob{params.JobHostUnits})

	err = s.machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Remove()
	c.Assert(err, IsNil)

	m, err = s.st.Agent().Entity(s.machine.Tag())
	c.Assert(err, ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(params.ErrCode(err), Equals, params.CodeNotFound)
	c.Assert(m, IsNil)
}

func (s *suite) TestEntitySetPassword(c *C) {
	entity, err := s.st.Agent().Entity(s.machine.Tag())
	c.Assert(err, IsNil)

	err = entity.SetPassword("foo")
	c.Assert(err, IsNil)

	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine.PasswordValid("bar"), Equals, false)
	c.Assert(s.machine.PasswordValid("foo"), Equals, true)

	// Check that we cannot log in to mongo with the wrong password.
	info := s.StateInfo(c)
	info.Tag = entity.Tag()
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
