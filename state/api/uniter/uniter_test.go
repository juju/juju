// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type uniterSuite struct {
	testing.JujuConnSuite
	st      *api.State
	machine *state.Machine
	service *state.Service
	unit    *state.Unit

	uniter *uniter.State
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine, a service and add a unit so we can log in as
	// its agent.
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.service, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	err = s.unit.SetPassword("password")
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), "password")

	// Create the uniter API facade.
	s.uniter = s.st.Uniter()
	c.Assert(s.uniter, gc.NotNil)
}

func (s *uniterSuite) TearDownTest(c *gc.C) {
	err := s.st.Close()
	c.Assert(err, gc.IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *uniterSuite) TestUnitAndUnitTag(c *gc.C) {
	unit, err := s.uniter.Unit("unit-foo-42")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
	c.Assert(unit, gc.IsNil)

	unit, err = s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Tag(), gc.Equals, "unit-wordpress-0")
}

func (s *uniterSuite) TestSetStatus(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	status, info, err := s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	err = unit.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, gc.IsNil)

	status, info, err = s.unit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
}

func (s *uniterSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.unit.Life(), gc.Equals, state.Alive)

	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)

	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)

	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dead)

	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dead)

	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	err = unit.EnsureDead()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeNotFound)
}

func (s *uniterSuite) TestRefresh(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	err = unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Dead)
}

func (s *uniterSuite) TestWatch(c *gc.C) {
	unit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Alive)

	w, err := unit.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = unit.SetStatus(params.StatusStarted, "not really")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the unit dead and check it's detected.
	err = unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
