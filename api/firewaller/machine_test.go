// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	s.apiMachine, err = s.firewaller.Machine(s.machines[0].Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine0, gc.NotNil)
}

func (s *machineSuite) TestTag(c *gc.C) {
	c.Assert(s.apiMachine.Tag(), gc.Equals, names.NewMachineTag(s.machines[0].Id()))
}

func (s *machineSuite) TestInstanceId(c *gc.C) {
	// Add another, not provisioned machine to test
	// CodeNotProvisioned.
	newMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	apiNewMachine, err := s.firewaller.Machine(newMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	_, err = apiNewMachine.InstanceId()
	c.Assert(err, gc.ErrorMatches, "machine 3 not provisioned")
	c.Assert(err, jc.Satisfies, params.IsCodeNotProvisioned)

	instanceId, err := s.apiMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-manager"))
}

func (s *machineSuite) TestWatchUnits(c *gc.C) {
	w, err := s.apiMachine.WatchUnits()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Unassign unit 0 from the machine and check it's detected.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress/0")
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *machineSuite) TestActiveNetworks(c *gc.C) {
	// No ports opened at first, no networks.
	nets, err := s.apiMachine.ActiveNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nets, gc.HasLen, 0)

	// Open a port and check again.
	err = s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, jc.ErrorIsNil)
	nets, err = s.apiMachine.ActiveNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nets, jc.DeepEquals, []names.NetworkTag{
		names.NewNetworkTag(network.DefaultPublic),
	})

	// Remove all ports, no networks.
	ports, err := s.machines[0].OpenedPorts(network.DefaultPublic)
	c.Assert(err, jc.ErrorIsNil)
	err = ports.Remove()
	c.Assert(err, jc.ErrorIsNil)
	nets, err = s.apiMachine.ActiveNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nets, gc.HasLen, 0)
}

func (s *machineSuite) TestOpenedPorts(c *gc.C) {
	networkTag := names.NewNetworkTag(network.DefaultPublic)
	unitTag := s.units[0].Tag().(names.UnitTag)

	// No ports opened at first.
	ports, err := s.apiMachine.OpenedPorts(networkTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	// Open a port and check again.
	err = s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, jc.ErrorIsNil)
	ports, err = s.apiMachine.OpenedPorts(networkTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, jc.DeepEquals, map[network.PortRange]names.UnitTag{
		network.PortRange{FromPort: 1234, ToPort: 1234, Protocol: "tcp"}: unitTag,
	})
}
