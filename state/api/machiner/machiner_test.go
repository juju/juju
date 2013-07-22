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
	statetesting "launchpad.net/juju-core/state/testing"
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
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, IsNil)
	err = s.machine.SetPassword("password")
	c.Assert(err, IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), "password", "fake_nonce")
}

func (s *machinerSuite) TearDownTest(c *C) {
	err := s.st.Close()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *machinerSuite) TestMachineAndMachineId(c *C) {
	machine, err := s.st.Machiner().Machine("machine-42")
	c.Assert(err, ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), Equals, params.CodeUnauthorized)
	c.Assert(machine, IsNil)

	machine, err = s.st.Machiner().Machine("machine-0")
	c.Assert(err, IsNil)
	c.Assert(machine.Tag(), Equals, "machine-0")
}

func (s *machinerSuite) TestSetStatus(c *C) {
	machine, err := s.st.Machiner().Machine("machine-0")
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

	machine, err := s.st.Machiner().Machine("machine-0")
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
	machine, err := s.st.Machiner().Machine("machine-0")
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Alive)

	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Alive)

	err = machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Dead)
}

func (s *machinerSuite) TestWatch(c *C) {
	machine, err := s.st.Machiner().Machine("machine-0")
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Alive)

	w, err := machine.Watch()
	c.Assert(err, IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = machine.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Make the machine dead and check it's detected.
	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
