// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type unitSuite struct {
	firewallerSuite

	apiUnit *firewaller.Unit
}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiUnit, err = s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)
}

func (s *unitSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *unitSuite) TestUnit(c *gc.C) {
	apiUnitFoo, err := s.firewaller.Unit(names.NewUnitTag("foo/42"))
	c.Assert(err, gc.ErrorMatches, `unit "foo/42" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiUnitFoo, gc.IsNil)

	apiUnit0, err := s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)
	c.Assert(apiUnit0, gc.NotNil)
	c.Assert(apiUnit0.Name(), gc.Equals, s.units[0].Name())
	c.Assert(apiUnit0.Tag(), gc.Equals, names.NewUnitTag(s.units[0].Name()))
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

func (s *unitSuite) TestWatchV0(c *gc.C) {
	s.patchNewState(c, firewaller.NewStateV0)

	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	w, err := s.apiUnit.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the life cycle and make sure it's
	// not detected.
	err = s.units[0].SetStatus(state.StatusStarted, "not really", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the unit dead and check it's detected.
	err = s.units[0].EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitSuite) TestWatchNotImplementedV1(c *gc.C) {
	s.patchNewState(c, firewaller.NewStateV1)

	w, err := s.apiUnit.Watch()
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
	c.Assert(err, gc.ErrorMatches, `unit.Watch\(\) \(in V1\+\) not implemented`)
	c.Assert(w, gc.IsNil)
}

func (s *unitSuite) TestAssignedMachine(c *gc.C) {
	machineTag, err := s.apiUnit.AssignedMachine()
	c.Assert(err, gc.IsNil)
	c.Assert(machineTag, gc.Equals, names.NewMachineTag(s.machines[0].Id()))

	// Unassign now and check CodeNotAssigned is reported.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	_, err = s.apiUnit.AssignedMachine()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not assigned to a machine`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotAssigned)
}

func (s *unitSuite) TestOpenedPortsV0(c *gc.C) {
	s.patchNewState(c, firewaller.NewStateV0)

	ports, err := s.apiUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, jc.DeepEquals, []network.Port{})

	// Open some ports and check again.
	err = s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("tcp", 4321)
	c.Assert(err, gc.IsNil)
	ports, err = s.apiUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(ports, jc.DeepEquals, []network.Port{{"tcp", 1234}, {"tcp", 4321}})
}

func (s *unitSuite) TestOpenedPortsNotImplementedV1(c *gc.C) {
	s.patchNewState(c, firewaller.NewStateV1)

	ports, err := s.apiUnit.OpenedPorts()
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
	c.Assert(err, gc.ErrorMatches, `unit.OpenedPorts\(\) \(in V1\+\) not implemented`)
	c.Assert(ports, gc.HasLen, 0)
}

func (s *unitSuite) TestService(c *gc.C) {
	service, err := s.apiUnit.Service()
	c.Assert(err, gc.IsNil)
	c.Assert(service.Name(), gc.Equals, s.service.Name())
}

func (s *unitSuite) TestName(c *gc.C) {
	c.Assert(s.apiUnit.Name(), gc.Equals, s.units[0].Name())
}

func (s *unitSuite) patchNewState(
	c *gc.C,
	patchFunc func(_ base.APICaller) *firewaller.State,
) {
	s.firewallerSuite.patchNewState(c, patchFunc)
	var err error
	s.apiUnit, err = s.firewaller.Unit(s.units[0].Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)
}
