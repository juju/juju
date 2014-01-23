// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/firewaller"
	"launchpad.net/juju-core/state/api/params"
	statetesting "launchpad.net/juju-core/state/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type unitSuite struct {
	firewallerSuite

	apiUnit *firewaller.Unit
}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiUnit, err = s.firewaller.Unit(s.units[0].Tag())
	c.Assert(err, gc.IsNil)
}

func (s *unitSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *unitSuite) TestUnit(c *gc.C) {
	apiUnitFoo, err := s.firewaller.Unit("unit-foo-42")
	c.Assert(err, gc.ErrorMatches, `unit "foo/42" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiUnitFoo, gc.IsNil)

	apiUnit0, err := s.firewaller.Unit(s.units[0].Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(apiUnit0, gc.NotNil)
	c.Assert(apiUnit0.Name(), gc.Equals, s.units[0].Name())
}

func (s *unitSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	err := s.units[0].EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	err = s.apiUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Dead)
}

func (s *unitSuite) TestWatch(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	w, err := s.apiUnit.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the life cycle and make sure it's
	// not detected.
	err = s.units[0].SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the unit dead and check it's detected.
	err = s.units[0].EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitSuite) TestAssignedMachine(c *gc.C) {
	machineTag, err := s.apiUnit.AssignedMachine()
	c.Assert(err, gc.IsNil)
	c.Assert(machineTag, gc.Equals, s.machines[0].Tag())

	// Unassign now and check CodeNotAssigned is reported.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	_, err = s.apiUnit.AssignedMachine()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotAssigned)
}

func (s *unitSuite) TestOpenedPorts(c *gc.C) {
	ports, err := s.apiUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, jc.DeepEquals, []instance.Port{})

	// Open some ports and check again.
	err = s.units[0].OpenPort("foo", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("bar", 4321)
	c.Assert(err, gc.IsNil)
	ports, err = s.apiUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, jc.DeepEquals, []instance.Port{{"bar", 4321}, {"foo", 1234}})
}

func (s *unitSuite) TestService(c *gc.C) {
	service, err := s.apiUnit.Service()
	c.Assert(err, gc.IsNil)
	c.Assert(service.Name(), gc.Equals, s.service.Name())
}

func (s *unitSuite) TestName(c *gc.C) {
	c.Assert(s.apiUnit.Name(), gc.Equals, s.units[0].Name())
}
