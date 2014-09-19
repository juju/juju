// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/firewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type firewallerV1Suite struct {
	firewallerBaseSuite
	*commontesting.EnvironWatcherTest

	firewaller *firewaller.FirewallerAPIV1
}

var _ = gc.Suite(&firewallerV1Suite{})

func (s *firewallerV1Suite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c)

	// Create a firewaller API V1 for the machine.
	firewallerAPI, err := firewaller.NewFirewallerAPIV1(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.firewaller = firewallerAPI
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(s.firewaller, s.State, s.resources, commontesting.HasSecrets)
}

func (s *firewallerV1Suite) TestFirewallerFailsWithNonEnvironManagerUser(c *gc.C) {
	constructor := func(st *state.State, res *common.Resources, auth common.Authorizer) error {
		_, err := firewaller.NewFirewallerAPIV1(st, res, auth)
		return err
	}
	s.testFirewallerFailsWithNonEnvironManagerUser(c, constructor)
}

func (s *firewallerV1Suite) TestLife(c *gc.C) {
	s.testLife(c, s.firewaller)
}

func (s *firewallerV1Suite) TestInstanceId(c *gc.C) {
	s.testInstanceId(c, s.firewaller)
}

func (s *firewallerV1Suite) TestWatchEnvironMachines(c *gc.C) {
	s.testWatchEnvironMachines(c, s.firewaller)
}

func (s *firewallerV1Suite) TestWatch(c *gc.C) {
	s.testWatch(c, s.firewaller, cannotWatchUnits)
}

func (s *firewallerV1Suite) TestWatchUnits(c *gc.C) {
	s.testWatchUnits(c, s.firewaller)
}

func (s *firewallerV1Suite) TestGetExposed(c *gc.C) {
	s.testGetExposed(c, s.firewaller)
}

func (s *firewallerV1Suite) TestOpenedPortsNotImplemented(c *gc.C) {
	s.assertNotImplemented(c, s.firewaller, "OpenedPorts")
}

func (s *firewallerV1Suite) TestGetAssignedMachine(c *gc.C) {
	s.testGetAssignedMachine(c, s.firewaller)
}

func (s *firewallerV1Suite) openPorts(c *gc.C) {
	// Open some ports on the units.
	err := s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("tcp", 4321)
	c.Assert(err, gc.IsNil)
	err = s.units[2].OpenPort("tcp", 1111)
	c.Assert(err, gc.IsNil)
}

func (s *firewallerV1Suite) TestWatchOpenedPorts(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	s.openPorts(c)
	expectChanges := []string{
		"m#0#n#juju-public",
		"m#2#n#juju-public",
	}

	fakeEnvTag := names.NewEnvironTag("deadbeef-deaf-face-feed-0123456789ab")
	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: fakeEnvTag.String()},
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.service.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	result, err := s.firewaller.WatchOpenedPorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Changes: expectChanges, StringsWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *firewallerV1Suite) TestGetMachinePorts(c *gc.C) {
	s.openPorts(c)

	networkTag := names.NewNetworkTag(network.DefaultPublic).String()
	args := params.MachinePortsParams{
		Params: []params.MachinePorts{
			{MachineTag: s.machines[0].Tag().String(), NetworkTag: networkTag},
			{MachineTag: s.machines[1].Tag().String(), NetworkTag: networkTag},
			{MachineTag: s.machines[2].Tag().String(), NetworkTag: networkTag},
			{MachineTag: s.machines[0].Tag().String(), NetworkTag: "invalid"},
			{MachineTag: "machine-42", NetworkTag: networkTag},
			{MachineTag: s.machines[0].Tag().String(), NetworkTag: "network-missing"},
		},
	}
	unit0Tag := s.units[0].Tag().String()
	expectPortsMachine0 := []params.MachinePortRange{
		{UnitTag: unit0Tag, PortRange: network.PortRange{
			FromPort: 1234, ToPort: 1234, Protocol: "tcp",
		}},
		{UnitTag: unit0Tag, PortRange: network.PortRange{
			FromPort: 4321, ToPort: 4321, Protocol: "tcp",
		}},
	}
	unit2Tag := s.units[2].Tag().String()
	expectPortsMachine2 := []params.MachinePortRange{
		{UnitTag: unit2Tag, PortRange: network.PortRange{
			FromPort: 1111, ToPort: 1111, Protocol: "tcp",
		}},
	}
	result, err := s.firewaller.GetMachinePorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.MachinePortsResults{
		Results: []params.MachinePortsResult{
			{Ports: expectPortsMachine0},
			{Error: nil, Ports: nil},
			{Ports: expectPortsMachine2},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: nil, Ports: nil},
		},
	})

}

func (s *firewallerV1Suite) TestGetMachineActiveNetworks(c *gc.C) {
	s.openPorts(c)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.service.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	networkTag := names.NewNetworkTag(network.DefaultPublic)
	expectResults := []string{networkTag.String()}
	result, err := s.firewaller.GetMachineActiveNetworks(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: expectResults},
			{Result: nil, Error: nil},
			{Result: expectResults},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
