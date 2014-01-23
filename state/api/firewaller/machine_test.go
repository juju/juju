// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/firewaller"
	"launchpad.net/juju-core/state/api/params"
	statetesting "launchpad.net/juju-core/state/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type machineSuite struct {
	firewallerSuite

	apiMachine *firewaller.Machine
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiMachine, err = s.firewaller.Machine(s.machines[0].Tag())
	c.Assert(err, gc.IsNil)
}

func (s *machineSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *machineSuite) TestMachine(c *gc.C) {
	apiMachine42, err := s.firewaller.Machine("machine-42")
	c.Assert(err, gc.ErrorMatches, "machine 42 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiMachine42, gc.IsNil)

	apiMachine0, err := s.firewaller.Machine(s.machines[0].Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine0, gc.NotNil)
}

func (s *machineSuite) TestInstanceId(c *gc.C) {
	// Add another, not provisioned machine to test
	// CodeNotProvisioned.
	newMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	apiNewMachine, err := s.firewaller.Machine(newMachine.Tag())
	c.Assert(err, gc.IsNil)
	_, err = apiNewMachine.InstanceId()
	c.Assert(err, gc.ErrorMatches, "machine 3 is not provisioned")
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)

	instanceId, err := s.apiMachine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-manager"))
}

func (s *machineSuite) TestWatchUnits(c *gc.C) {
	w, err := s.apiMachine.WatchUnits()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange("wordpress/0")
	wc.AssertNoChange()

	// Change something other than the life cycle and make sure it's
	// not detected.
	err = s.machines[0].SetPassword("foo")
	c.Assert(err, gc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	wc.AssertNoChange()

	err = s.machines[0].SetPassword("foo-12345678901234567890")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Unassign unit 0 from the machine and check it's detected.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("wordpress/0")
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
