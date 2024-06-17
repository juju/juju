// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"
	"sort"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type firewallerSuite struct {
	firewallerBaseSuite

	firewaller *firewaller.FirewallerAPI

	watcherRegistry     *facademocks.MockWatcherRegistry
	controllerConfigAPI *MockControllerConfigAPI

	controllerConfigService *MockControllerConfigService
	modelConfigService      *MockModelConfigService
	networkService          *MockNetworkService
	machineService          *MockMachineService
}

var _ = gc.Suite(&firewallerSuite{})

func (s *firewallerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.controllerConfigAPI = NewMockControllerConfigAPI(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)

	return ctrl
}

func (s *firewallerSuite) setupAPI(c *gc.C) {
	st := s.ControllerModel(c).State()

	serviceFactory := s.ControllerServiceFactory(c)

	cloudSpecAPI := cloudspec.NewCloudSpec(
		s.resources,
		cloudspec.MakeCloudSpecGetterForModel(st, serviceFactory.Cloud(), serviceFactory.Credential()),
		cloudspec.MakeCloudSpecWatcherForModel(st, serviceFactory.Cloud()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, serviceFactory.Credential()),
		common.AuthFuncForTag(s.ControllerModel(c).ModelTag()),
	)

	// Create a firewaller API for the machine.
	firewallerAPI, err := firewaller.NewStateFirewallerAPI(
		firewaller.StateShim(st, s.ControllerModel(c)),
		s.networkService,
		s.resources,
		s.watcherRegistry,
		s.authorizer,
		cloudSpecAPI,
		s.controllerConfigAPI,
		s.controllerConfigService,
		s.modelConfigService,
		s.machineService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.firewaller = firewallerAPI
}

func (s *firewallerSuite) TestFirewallerFailsWithNonControllerUser(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	constructor := func(ctx facade.ModelContext) error {
		_, err := firewaller.NewFirewallerAPIV7(ctx)
		return err
	}
	s.testFirewallerFailsWithNonControllerUser(c, constructor)
}

func (s *firewallerSuite) TestLife(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.testLife(c, s.firewaller)
}

func (s *firewallerSuite) TestInstanceId(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.testInstanceId(c, s.firewaller)
}

func (s *firewallerSuite) TestWatchModelMachines(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	changes := make(chan []string, 3)
	// Simulate initial event.
	changes <- []string{"0", "1", "2"}
	stringsWatcher := watchertest.NewMockStringsWatcher(changes)
	s.machineService.EXPECT().WatchModelMachines(gomock.Any()).Return(
		stringsWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	s.testWatchModelMachines(c, s.firewaller)
}

func (s *firewallerSuite) TestWatch(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.testWatch(c, s.firewaller, cannotWatchUnits)
}

func (s *firewallerSuite) TestWatchUnits(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.testWatchUnits(c, s.firewaller)
}

func (s *firewallerSuite) TestGetAssignedMachine(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

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

	c.Assert(s.ControllerModel(c).State().ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}

func (s *firewallerSuite) TestWatchOpenedPorts(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

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
	result, err := s.firewaller.WatchOpenedPorts(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *firewallerSuite) TestAreManuallyProvisioned(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	st := s.ControllerModel(c).State()
	m, err := st.AddOneMachine(s.InstancePrechecker(c, st), state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
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

	result, err := s.firewaller.AreManuallyProvisioned(context.Background(), args)
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
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	// Set the application to exposed first.
	err := s.application.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"10.0.0.0/0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	args := addFakeEntities(params.Entities{Entities: []params.Entity{
		{Tag: s.application.Tag().String()},
	}})
	result, err := s.firewaller.GetExposeInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ExposeInfoResults{
		Results: []params.ExposeInfoResult{
			{
				Exposed: true,
				ExposedEndpoints: map[string]params.ExposedEndpoint{
					"": {
						ExposeToSpaces: []string{network.AlphaSpaceId},
						ExposeToCIDRs:  []string{"10.0.0.0/0"},
					},
				},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`application "bar"`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now reset the exposed flag for the application and check again.
	err = s.application.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	args = params.Entities{Entities: []params.Entity{
		{Tag: s.application.Tag().String()},
	}}
	result, err = s.firewaller.GetExposeInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ExposeInfoResults{
		Results: []params.ExposeInfoResult{
			{Exposed: false},
		},
	})
}
