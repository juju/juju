// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type machinerSuite struct {
	testing.JujuConnSuite
	st      *api.State
	machine *state.Machine
}

var _ = Suite(&machinerSuite{})

func (s *machinerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine so we can log in as its agent.
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machine.SetPassword("password")
	s.st = s.OpenAPIAs(c, s.machine.Tag(), "password")
}

func (s *machinerSuite) TearDownTest(c *C) {
	err := s.st.Close()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *machinerSuite) TestMachineAndMachineId(c *C) {
	machine, err := s.st.Machiner().Machine("42")
	c.Assert(err, ErrorMatches, "machine 42 not found")
	c.Assert(params.ErrCode(err), Equals, params.CodeNotFound)
	c.Assert(machine, IsNil)

	machine, err = s.st.Machiner().Machine("0")
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, "0")
}

func (s *machinerSuite) TestSetStatus(c *C) {
	machine, err := s.st.Machiner().Machine("0")
	c.Assert(err, IsNil)

	status, info, err := s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	err = machine.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, IsNil)

	status, info, err = s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStarted)
	c.Assert(info, Equals, "blah")
}

func (s *machinerSuite) TestEnsureDead(c *C) {
	c.Assert(s.machine.Life(), Equals, state.Alive)

	machine, err := s.st.Machiner().Machine("0")
	c.Assert(err, IsNil)

	err = machine.EnsureDead()
	c.Assert(err, IsNil)

	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine.Life(), Equals, state.Dead)

	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine.Life(), Equals, state.Dead)

	err = s.machine.Remove()
	c.Assert(err, IsNil)
	err = s.machine.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	err = machine.EnsureDead()
	c.Assert(err, ErrorMatches, "machine 0 not found")
	c.Assert(params.ErrCode(err), Equals, params.CodeNotFound)
}

func (s *machinerSuite) TestRefresh(c *C) {
	machine, err := s.st.Machiner().Machine("0")
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Alive)

	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Alive)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Dead)
}
