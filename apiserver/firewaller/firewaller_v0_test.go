// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/firewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type firewallerV0Suite struct {
	firewallerBaseSuite
	*commontesting.EnvironWatcherTest

	firewaller *firewaller.FirewallerAPIV0
}

var _ = gc.Suite(&firewallerV0Suite{})

func (s *firewallerV0Suite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c)

	// Create a firewaller API V0 for the machine.
	firewallerAPI, err := firewaller.NewFirewallerAPIV0(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.firewaller = firewallerAPI
	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(s.firewaller, s.State, s.resources, commontesting.HasSecrets)
}

func (s *firewallerV0Suite) TestFirewallerFailsWithNonEnvironManagerUser(c *gc.C) {
	constructor := func(st *state.State, res *common.Resources, auth common.Authorizer) error {
		_, err := firewaller.NewFirewallerAPIV0(st, res, auth)
		return err
	}
	s.testFirewallerFailsWithNonEnvironManagerUser(c, constructor)
}

func (s *firewallerV0Suite) TestLife(c *gc.C) {
	s.testLife(c, s.firewaller)
}

func (s *firewallerV0Suite) TestInstanceId(c *gc.C) {
	s.testInstanceId(c, s.firewaller)
}

func (s *firewallerV0Suite) TestWatchEnvironMachines(c *gc.C) {
	s.testWatchEnvironMachines(c, s.firewaller)
}

func (s *firewallerV0Suite) TestWatch(c *gc.C) {
	s.testWatch(c, s.firewaller, canWatchUnits)
}

func (s *firewallerV0Suite) TestWatchUnits(c *gc.C) {
	s.testWatchUnits(c, s.firewaller)
}

func (s *firewallerV0Suite) TestGetExposed(c *gc.C) {
	s.testGetExposed(c, s.firewaller)
}

func (s *firewallerV0Suite) TestOpenedPorts(c *gc.C) {
	// Open some ports on two of the units.
	err := s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[0].OpenPort("tcp", 4321)
	c.Assert(err, gc.IsNil)
	err = s.units[2].OpenPort("tcp", 1111)
	c.Assert(err, gc.IsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.units[0].Tag().String()},
		{Tag: s.units[1].Tag().String()},
		{Tag: s.units[2].Tag().String()},
	}})
	result, err := s.firewaller.OpenedPorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.PortsResults{
		Results: []params.PortsResult{
			{Ports: []network.Port{{"tcp", 1234}, {"tcp", 4321}}},
			{Ports: []network.Port{}},
			{Ports: []network.Port{{"tcp", 1111}}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "foo/0"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now close unit 2's port and check again.
	err = s.units[2].ClosePort("tcp", 1111)
	c.Assert(err, gc.IsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.units[2].Tag().String()},
	}}
	result, err = s.firewaller.OpenedPorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.PortsResults{
		Results: []params.PortsResult{
			{Ports: []network.Port{}},
		},
	})
}

func (s *firewallerV0Suite) TestWatchOpenedPortsNotImplemented(c *gc.C) {
	s.assertNotImplemented(c, s.firewaller, "WatchOpenedPorts")
}

func (s *firewallerV0Suite) TestGetMachineActiveNetworksNotImplemented(c *gc.C) {
	s.assertNotImplemented(c, s.firewaller, "GetMachineActiveNetworks")
}

func (s *firewallerV0Suite) TestGetMachinePortsNotImplemented(c *gc.C) {
	s.assertNotImplemented(c, s.firewaller, "GetMachinePorts")
}

func (s *firewallerV0Suite) TestGetAssignedMachine(c *gc.C) {
	s.testGetAssignedMachine(c, s.firewaller)
}
