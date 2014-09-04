// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type machineSuite struct {
	firewallerSuite

	apiMachine *firewaller.Machine
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	// TODO(dfc) fix this
	s.apiMachine, err = s.firewaller.Machine(s.machines[0].Tag().(names.MachineTag))
	c.Assert(err, gc.IsNil)
}

func (s *machineSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *machineSuite) TestMachine(c *gc.C) {
	apiMachine42, err := s.firewaller.Machine(names.NewMachineTag("42"))
	c.Assert(err, gc.ErrorMatches, "machine 42 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiMachine42, gc.IsNil)

	apiMachine0, err := s.firewaller.Machine(s.machines[0].Tag().(names.MachineTag))
	c.Assert(err, gc.IsNil)
	c.Assert(apiMachine0, gc.NotNil)
}

func (s *machineSuite) TestInstanceId(c *gc.C) {
	// Add another, not provisioned machine to test
	// CodeNotProvisioned.
	newMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	apiNewMachine, err := s.firewaller.Machine(newMachine.Tag().(names.MachineTag))
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

func (s *machineSuite) TestActiveNetworks(c *gc.C) {
	networkNames, err := s.apiMachine.ActiveNetworks()
	c.Assert(err, gc.IsNil)
	c.Assert(networkNames, gc.HasLen, 0)

	// Open some ports and check again.
	err = s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("tcp", 4321)
	c.Assert(err, gc.IsNil)
	networkNames, err = s.apiMachine.ActiveNetworks()
	c.Assert(err, gc.IsNil)
	c.Assert(networkNames, gc.HasLen, 1)
	c.Assert(networkNames[0], gc.Equals, network.DefaultPublic)
}

func (s *machineSuite) TestGetPorts(c *gc.C) {
	ports, err := s.apiMachine.GetPorts(names.NewNetworkTag(network.DefaultPublic))
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 0)

	// Open some ports and check again.
	err = s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("tcp", 4321)
	c.Assert(err, gc.IsNil)
	ports, err = s.apiMachine.GetPorts(names.NewNetworkTag(network.DefaultPublic))
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.HasLen, 2)

}
