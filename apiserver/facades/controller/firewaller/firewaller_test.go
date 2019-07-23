// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"sort"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type firewallerSuite struct {
	firewallerBaseSuite
	*commontesting.ModelWatcherTest

	firewaller *firewaller.FirewallerAPIV3
}

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c)

	_, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.20.30.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	cloudSpecAPI := cloudspec.NewCloudSpec(
		s.resources,
		cloudspec.MakeCloudSpecGetterForModel(s.State),
		cloudspec.MakeCloudSpecWatcherForModel(s.State),
		common.AuthFuncForTag(s.Model.ModelTag()),
	)
	// Create a firewaller API for the machine.
	firewallerAPI, err := firewaller.NewFirewallerAPI(
		firewaller.StateShim(s.State, s.Model),
		s.resources,
		s.authorizer,
		cloudSpecAPI,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.firewaller = firewallerAPI
	s.ModelWatcherTest = commontesting.NewModelWatcherTest(s.firewaller, s.State, s.resources)
}

func (s *firewallerSuite) TestFirewallerFailsWithNonControllerUser(c *gc.C) {
	constructor := func(st *state.State, res facade.Resources, auth facade.Authorizer) error {
		m, err := st.Model()
		c.Assert(err, jc.ErrorIsNil)

		_, err = firewaller.NewFirewallerAPI(firewaller.StateShim(st, m), res, auth, nil)
		return err
	}
	s.testFirewallerFailsWithNonControllerUser(c, constructor)
}

func (s *firewallerSuite) TestLife(c *gc.C) {
	s.testLife(c, s.firewaller)
}

func (s *firewallerSuite) TestInstanceId(c *gc.C) {
	s.testInstanceId(c, s.firewaller)
}

func (s *firewallerSuite) TestWatchModelMachines(c *gc.C) {
	s.testWatchModelMachines(c, s.firewaller)
}

func (s *firewallerSuite) TestWatch(c *gc.C) {
	s.testWatch(c, s.firewaller, cannotWatchUnits)
}

func (s *firewallerSuite) TestWatchUnits(c *gc.C) {
	s.testWatchUnits(c, s.firewaller)
}

func (s *firewallerSuite) TestGetExposed(c *gc.C) {
	s.testGetExposed(c, s.firewaller)
}

func (s *firewallerSuite) TestGetAssignedMachine(c *gc.C) {
	s.testGetAssignedMachine(c, s.firewaller)
}

func (s *firewallerSuite) openPorts(c *gc.C) {
	// Open some ports on the units.
	err := s.units[0].OpenPortsOnSubnet("10.20.30.0/24", "tcp", 1234, 1400)
	c.Assert(err, jc.ErrorIsNil)
	err = s.units[0].OpenPort("tcp", 4321)
	c.Assert(err, jc.ErrorIsNil)
	err = s.units[2].OpenPorts("udp", 1111, 2222)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *firewallerSuite) TestWatchOpenedPorts(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	s.openPorts(c)
	expectChanges := []string{
		"0:", // empty subnet is ok (until it can be made mandatory)
		"0:10.20.30.0/24",
		"2:",
	}

	fakeModelTag := names.NewModelTag("deadbeef-deaf-face-feed-0123456789ab")
	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: fakeModelTag.String()},
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.application.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	result, err := s.firewaller.WatchOpenedPorts(args)
	sort.Strings(result.Results[0].Changes)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *firewallerSuite) TestGetMachinePorts(c *gc.C) {
	s.openPorts(c)

	subnetTag := names.NewSubnetTag("10.20.30.0/24").String()
	args := params.MachinePortsParams{
		Params: []params.MachinePorts{
			{MachineTag: s.machines[0].Tag().String(), SubnetTag: ""},
			{MachineTag: s.machines[0].Tag().String(), SubnetTag: subnetTag},
			{MachineTag: s.machines[1].Tag().String(), SubnetTag: ""},
			{MachineTag: s.machines[2].Tag().String(), SubnetTag: ""},
			{MachineTag: s.machines[0].Tag().String(), SubnetTag: "invalid"},
			{MachineTag: "machine-42", SubnetTag: ""},
			{MachineTag: s.machines[0].Tag().String(), SubnetTag: "subnet-bad"},
		},
	}
	unit0Tag := s.units[0].Tag().String()
	expectPortsMachine0NoSubnet := []params.MachinePortRange{
		{UnitTag: unit0Tag, PortRange: params.PortRange{
			FromPort: 4321, ToPort: 4321, Protocol: "tcp",
		}},
	}
	expectPortsMachine0WithSubnet := []params.MachinePortRange{
		{UnitTag: unit0Tag, PortRange: params.PortRange{
			FromPort: 1234, ToPort: 1400, Protocol: "tcp",
		}},
	}
	unit2Tag := s.units[2].Tag().String()
	expectPortsMachine2 := []params.MachinePortRange{
		{UnitTag: unit2Tag, PortRange: params.PortRange{
			FromPort: 1111, ToPort: 2222, Protocol: "udp",
		}},
	}
	result, err := s.firewaller.GetMachinePorts(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.MachinePortsResults{
		Results: []params.MachinePortsResult{
			{Ports: expectPortsMachine0NoSubnet},
			{Ports: expectPortsMachine0WithSubnet},
			{Error: nil, Ports: nil},
			{Ports: expectPortsMachine2},
			{Error: apiservertesting.ServerError(`"invalid" is not a valid tag`)},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"subnet-bad" is not a valid subnet tag`)},
		},
	})

}

func (s *firewallerSuite) TestGetMachineActiveSubnets(c *gc.C) {
	s.openPorts(c)

	subnetTag := names.NewSubnetTag("10.20.30.0/24").String()
	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: s.machines[2].Tag().String()},
		{Tag: s.application.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})
	expectResultsMachine0 := []string{subnetTag, ""}
	expectResultsMachine2 := []string{""}
	result, err := s.firewaller.GetMachineActiveSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: expectResultsMachine0},
			{Result: nil, Error: nil},
			{Result: expectResultsMachine2},
			{Error: apiservertesting.ServerError(`"application-wordpress" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"unit-wordpress-0" is not a valid machine tag`)},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"unit-foo-0" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"application-bar" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"user-foo" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"foo-bar" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
		},
	})
}

func (s *firewallerSuite) TestAreManuallyProvisioned(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:     "quantal",
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "2",
		Nonce:      "manual:",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag().String()},
		{Tag: s.machines[1].Tag().String()},
		{Tag: m.Tag().String()},
		{Tag: s.application.Tag().String()},
		{Tag: s.units[0].Tag().String()},
	}})

	apiv5 := &firewaller.FirewallerAPIV5{
		&firewaller.FirewallerAPIV4{
			FirewallerAPIV3:     s.firewaller,
			ControllerConfigAPI: common.NewControllerConfig(newMockState(coretesting.ModelTag.Id())),
		}}

	result, err := apiv5.AreManuallyProvisioned(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: false, Error: nil},
			{Result: false, Error: nil},
			{Result: true, Error: nil},
			{Result: false, Error: apiservertesting.ServerError(`"application-wordpress" is not a valid machine tag`)},
			{Result: false, Error: apiservertesting.ServerError(`"unit-wordpress-0" is not a valid machine tag`)},
			{Result: false, Error: apiservertesting.NotFoundError("machine 42")},
			{Result: false, Error: apiservertesting.ServerError(`"unit-foo-0" is not a valid machine tag`)},
			{Result: false, Error: apiservertesting.ServerError(`"application-bar" is not a valid machine tag`)},
			{Result: false, Error: apiservertesting.ServerError(`"user-foo" is not a valid machine tag`)},
			{Result: false, Error: apiservertesting.ServerError(`"foo-bar" is not a valid tag`)},
			{Result: false, Error: apiservertesting.ServerError(`"" is not a valid tag`)},
		},
	})
}
