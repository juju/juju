// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/common/cloudspec"
	commontesting "github.com/juju/juju/v2/apiserver/common/testing"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/apiserver/facades/controller/firewaller"
	"github.com/juju/juju/v2/apiserver/facades/controller/firewaller/mocks"
	apiservertesting "github.com/juju/juju/v2/apiserver/testing"
	"github.com/juju/juju/v2/core/network"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state"
	statetesting "github.com/juju/juju/v2/state/testing"
)

type firewallerSuite struct {
	firewallerBaseSuite
	*commontesting.ModelWatcherTest

	cc         *mocks.MockControllerConfigAPI
	firewaller *firewaller.FirewallerAPIV3
	subnet     *state.Subnet
}

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.setUpTest(c)

	subnet, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "10.20.30.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	s.subnet = subnet

	cloudSpecAPI := cloudspec.NewCloudSpec(
		s.resources,
		cloudspec.MakeCloudSpecGetterForModel(s.State),
		cloudspec.MakeCloudSpecWatcherForModel(s.State),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(s.State),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(s.State),
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

func (s *firewallerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.cc = mocks.NewMockControllerConfigAPI(ctrl)
	return ctrl
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

func (s *firewallerSuite) TestGetAssignedMachine(c *gc.C) {
	s.testGetAssignedMachine(c, s.firewaller)
}

func (s *firewallerSuite) openPorts(c *gc.C) {
	// Open some ports on the units.
	allEndpoints := ""
	s.mustOpenPorts(c, s.units[0], allEndpoints, []network.PortRange{
		network.MustParsePortRange("1234-1400/tcp"),
		network.MustParsePortRange("4321/tcp"),
	})
	s.mustOpenPorts(c, s.units[2], allEndpoints, []network.PortRange{
		network.MustParsePortRange("1111-2222/udp"),
	})
}

func (s *firewallerSuite) mustOpenPorts(c *gc.C, unit *state.Unit, endpointName string, portRanges []network.PortRange) {
	unitPortRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	for _, pr := range portRanges {
		unitPortRanges.Open(endpointName, pr)
	}

	c.Assert(s.State.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}

func (s *firewallerSuite) TestWatchOpenedPorts(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	s.openPorts(c)
	expectChanges := []string{ // machine IDs
		"0",
		"2",
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

func (s *firewallerSuite) TestAreManuallyProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

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
			ControllerConfigAPI: s.cc,
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

func (s *firewallerSuite) TestGetExposeInfo(c *gc.C) {
	defer s.setup(c).Finish()

	apiv6 := &firewaller.FirewallerAPIV6{
		&firewaller.FirewallerAPIV5{
			&firewaller.FirewallerAPIV4{
				FirewallerAPIV3:     s.firewaller,
				ControllerConfigAPI: s.cc,
			},
		},
	}
	s.testGetExposeInfo(c, apiv6)
}

func (s *firewallerSuite) TestWatchSubnets(c *gc.C) {
	defer s.setup(c).Finish()

	// Set up a spaces with two subnets
	sp, err := s.State.AddSpace("outer-space", network.Id("outer-1"), nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{
		CIDR:      "192.168.0.0/24",
		SpaceID:   sp.Id(),
		SpaceName: sp.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	sub2, err := s.State.AddSubnet(network.SubnetInfo{
		CIDR:      "192.168.42.0/24",
		SpaceID:   sp.Id(),
		SpaceName: sp.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	apiv6 := &firewaller.FirewallerAPIV6{
		&firewaller.FirewallerAPIV5{
			&firewaller.FirewallerAPIV4{
				FirewallerAPIV3:     s.firewaller,
				ControllerConfigAPI: s.cc,
			},
		},
	}

	s.testWatchSubnets(
		c,
		[]names.SubnetTag{
			names.NewSubnetTag(sub2.ID()),
		},
		// We should only get sub2 in the initial changeset due to the
		// filter condition.
		[]string{sub2.ID()},
		apiv6,
	)
}
