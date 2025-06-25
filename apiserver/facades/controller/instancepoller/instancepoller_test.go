// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/instancepoller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type InstancePollerSuite struct {
	testhelpers.IsolationSuite

	st         *mockState
	api        *instancepoller.InstancePollerAPI
	authoriser apiservertesting.FakeAuthorizer
	resources  *common.Resources

	controllerConfigService *MockControllerConfigService
	networkService          *MockNetworkService
	machineService          *MockMachineService
	statusService           *MockStatusService

	machineEntities     params.Entities
	machineErrorResults params.ErrorResults

	mixedEntities     params.Entities
	mixedErrorResults params.ErrorResults

	clock clock.Clock
}

func TestInstancePollerSuite(t *testing.T) {
	tc.Run(t, &InstancePollerSuite{})
}

func (s *InstancePollerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	c.Cleanup(func() {
		s.controllerConfigService = nil
		s.networkService = nil
		s.machineService = nil
		s.statusService = nil
	})

	return ctrl
}

func (s *InstancePollerSuite) setupAPI(c *tc.C) (err error) {
	s.api, err = instancepoller.NewInstancePollerAPI(
		nil,
		nil,
		s.networkService,
		s.machineService,
		s.statusService,
		nil,
		s.resources,
		s.authoriser,
		s.controllerConfigService,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	return err
}

func (s *InstancePollerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })

	s.st = NewMockState()
	instancepoller.PatchState(s, s.st)

	s.clock = testclock.NewClock(time.Now())

	s.machineEntities = params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-2"},
			{Tag: "machine-3"},
		}}
	s.machineErrorResults = params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}}

	s.mixedEntities = params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-2"},
			{Tag: "machine-42"},
			{Tag: "application-unknown"},
			{Tag: "invalid-tag"},
			{Tag: "unit-missing-1"},
			{Tag: ""},
			{Tag: "42"},
		}}
	s.mixedErrorResults = params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}}
}

func (s *InstancePollerSuite) TestNewInstancePollerAPIRequiresController(c *tc.C) {
	s.authoriser.Controller = false

	err := s.setupAPI(c)
	c.Assert(s.api, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *InstancePollerSuite) TestWatchModelMachinesFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.assertMachineWatcherFails(c, "WatchModelMachines", s.api.WatchModelMachines)
}

func (s *InstancePollerSuite) TestWatchModelMachinesSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.assertMachineWatcherSucceeds(c, "WatchModelMachines", s.api.WatchModelMachines)
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.assertMachineWatcherFails(c, "WatchModelMachineStartTimes", s.api.WatchModelMachineStartTimes)
}

func (s *InstancePollerSuite) TestWatchModelMachineStartTimesSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.assertMachineWatcherFails(c, "WatchModelMachineStartTimes", s.api.WatchModelMachineStartTimes)
}

func (s *InstancePollerSuite) assertMachineWatcherFails(c *tc.C, watchFacadeName string, getWatcherFn func(context.Context) (params.StringsWatchResult, error)) {
	// Force the Changes() method of the mock watcher to return a
	// closed channel by setting an error.
	s.st.SetErrors(errors.Errorf("boom"))

	result, err := getWatcherFn(c.Context())
	c.Assert(err, tc.ErrorMatches, "cannot obtain initial model machines: boom")
	c.Assert(result, tc.DeepEquals, params.StringsWatchResult{})

	c.Assert(s.resources.Count(), tc.Equals, 0) // no watcher registered
	s.st.CheckCallNames(c, watchFacadeName)
}

func (s *InstancePollerSuite) assertMachineWatcherSucceeds(c *tc.C, watchFacadeName string, getWatcherFn func(context.Context) (params.StringsWatchResult, error)) {
	// Add a couple of machines.
	s.st.SetMachineInfo(c, machineInfo{id: "2"})
	s.st.SetMachineInfo(c, machineInfo{id: "1"})

	expectedResult := params.StringsWatchResult{
		Error:            nil,
		StringsWatcherId: "1",
		Changes:          []string{"1", "2"}, // initial event (sorted ids)
	}
	result, err := getWatcherFn(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expectedResult)

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource1 := s.resources.Get("1")
	defer func() {
		if resource1 != nil {
			workertest.CleanKill(c, resource1)
		}
	}()

	// Check that the watcher has consumed the initial event
	wc1 := watchertest.NewStringsWatcherC(c, resource1.(state.StringsWatcher))
	wc1.AssertNoChange()

	s.st.CheckCallNames(c, watchFacadeName)

	// Add another watcher to verify events coalescence.
	result, err = getWatcherFn(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	expectedResult.StringsWatcherId = "2"
	c.Assert(result, tc.DeepEquals, expectedResult)
	s.st.CheckCallNames(c, watchFacadeName, watchFacadeName)
	c.Assert(s.resources.Count(), tc.Equals, 2)
	resource2 := s.resources.Get("2")
	defer workertest.CleanKill(c, resource2)
	wc2 := watchertest.NewStringsWatcherC(c, resource2.(state.StringsWatcher))
	wc2.AssertNoChange()

	// Remove machine 1, check it's reported.
	s.st.RemoveMachine(c, "1")
	wc1.AssertChangeInSingleEvent("1")

	// Make separate changes, check they're combined.
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dying})
	s.st.SetMachineInfo(c, machineInfo{id: "3"})
	s.st.RemoveMachine(c, "42") // ignored
	wc1.AssertChangeInSingleEvent("2", "3")
	wc2.AssertChangeInSingleEvent("1", "2", "3")

	// Stop the first watcher and assert its changes chan is closed.
	resource1.Kill()
	c.Assert(resource1.Wait(), tc.ErrorIsNil)
	wc1.AssertKilled()
	resource1 = nil
}

func (s *InstancePollerSuite) TestLifeSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	exp := s.machineService.EXPECT()
	exp.GetMachineLife(gomock.Any(), machine.Name("1")).Return(life.Alive, nil)
	exp.GetMachineLife(gomock.Any(), machine.Name("2")).Return(life.Dying, nil)
	exp.GetMachineLife(gomock.Any(), machine.Name("42")).Return("", machineerrors.MachineNotFound)

	result, err := s.api.Life(c.Context(), s.mixedEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Dying},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}},
	)
}

func (s *InstancePollerSuite) TestLifeFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	exp := s.machineService.EXPECT()
	exp.GetMachineLife(gomock.Any(), machine.Name("1")).Return("", errors.New("pow!"))
	exp.GetMachineLife(gomock.Any(), machine.Name("2")).Return(life.Dead, nil)
	exp.GetMachineLife(gomock.Any(), machine.Name("3")).Return("", machineerrors.NotProvisioned)

	result, err := s.api.Life(c.Context(), s.machineEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Life: life.Dead},
			{Error: apiservertesting.NotProvisionedError("3")},
		}},
	)
}

func (s *InstancePollerSuite) TestInstanceIdSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-1")).Return("i-foo", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("uuid-2", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-2")).Return("", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("42")).Return("", machineerrors.MachineNotFound)

	result, err := s.api.InstanceId(c.Context(), s.mixedEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "i-foo"},
			{Result: ""},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}},
	)
}

func (s *InstancePollerSuite) TestInstanceIdFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("", errors.New("pow!"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("uuid-2", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-2")).Return("i-2", errors.New("FAIL"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("3")).Return("uuid-3", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-3")).Return("", machineerrors.NotProvisioned)
	result, err := s.api.InstanceId(c.Context(), s.machineEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("3")},
		}},
	)
}

func (s *InstancePollerSuite) TestStatusSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	now := s.clock.Now()

	s.statusService.EXPECT().GetMachineStatus(gomock.Any(), machine.Name("1")).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}, nil)

	result, err := s.api.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-1",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]interface{}{"foo": "bar"},
			Since:  &now,
		}},
	})
}

func (s *InstancePollerSuite) TestStatusNotFound(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.statusService.EXPECT().GetMachineStatus(gomock.Any(), machine.Name("1")).Return(status.StatusInfo{}, machineerrors.MachineNotFound)

	result, err := s.api.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-1",
	}}})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *InstancePollerSuite) TestStatusInvalidTags(c *tc.C) {
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.Status(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{Results: []params.StatusResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *InstancePollerSuite) TestInstanceStatusSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	now := s.clock.Now()

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), machine.Name("1")).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}, nil)

	result, err := s.api.InstanceStatus(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "machine-1"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]interface{}{"foo": "bar"},
			Since:  &now,
		}},
	})
}

func (s *InstancePollerSuite) TestInstanceStatusNotFound(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), machine.Name("1")).Return(status.StatusInfo{}, machineerrors.MachineNotFound)

	result, err := s.api.InstanceStatus(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "machine-1"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *InstancePollerSuite) TestInstanceStatusInvalidTags(c *tc.C) {
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.InstanceStatus(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{Results: []params.StatusResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *InstancePollerSuite) TestSetInstanceStatusSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	now := s.clock.Now()

	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), machine.Name("1"), status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}).Return(nil)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-1",
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]interface{}{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *InstancePollerSuite) TestSetInstanceStatusToError(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	now := s.clock.Now()

	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), machine.Name("0"), status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "blah",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}).Return(nil)
	s.statusService.EXPECT().SetMachineStatus(gomock.Any(), machine.Name("0"), status.StatusInfo{
		Status:  status.Error,
		Message: "blah",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}).Return(nil)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: status.ProvisioningError.String(),
			Info:   "blah",
			Data:   map[string]interface{}{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *InstancePollerSuite) TestSetInstanceStatusMachineNotFound(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	now := s.clock.Now()

	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), machine.Name("1"), status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}).Return(machineerrors.MachineNotFound)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-1",
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]interface{}{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *InstancePollerSuite) TestSetInstanceStatusInvalidTags(c *tc.C) {
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *InstancePollerSuite) TestAreManuallyProvisionedSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.machineService.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machine.Name("1")).Return(true, nil)
	s.machineService.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machine.Name("2")).Return(false, nil)
	s.machineService.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machine.Name("42")).Return(false, machineerrors.MachineNotFound)

	result, err := s.api.AreManuallyProvisioned(c.Context(), s.mixedEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: true},
			{Result: false},
			{Error: apiservertesting.NotFoundError(`machine "42"`)},
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.expectDefaultSpaces()

	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujutesting.FakeControllerConfig(), nil)
	s.machineService.EXPECT().GetMachineUUID(c.Context(), machine.Name("1")).Return("machine-uuid", nil)
	s.networkService.EXPECT().SetProviderNetConfig(c.Context(), machine.UUID("machine-uuid"),
		[]domainnetwork.NetInterface{{
			IsAutoStart: true,
			IsEnabled:   true,
			Addrs: []domainnetwork.NetAddr{
				{
					AddressValue: "10.0.0.42",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				{
					AddressValue: "10.73.37.110",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				{
					AddressValue: "10.73.37.111",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				{
					AddressValue: "192.168.0.1",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				// Shadows
				{
					AddressValue: "1.1.1.42",
					Scope:        "public",
					Origin:       "provider",
					IsShadow:     true,
				},
			}},
		}).Return(nil)

	results, err := s.api.SetProviderNetworkConfig(c.Context(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						Addresses: []params.Address{
							{
								Value: "10.0.0.42",
								Scope: "local-cloud",
								CIDR:  "10.0.0.0/24",
							},
							{
								Value: "10.73.37.110",
								Scope: "local-cloud",
								CIDR:  "10.73.37.0/24",
							},
							// Resolved by provider's space ID.
							{
								Value:           "10.73.37.111",
								Scope:           "local-cloud",
								ProviderSpaceID: "my-space-on-maas",
							},
							// This address does not match any of the CIDRs in
							// the known spaces; we expect this to end up in
							// alpha space.
							{
								Value: "192.168.0.1",
								Scope: "local-cloud",
							},
						},
						ShadowAddresses: []params.Address{
							{
								Value: "1.1.1.42",
								Scope: "public",
							},
						},
					},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Modified, tc.IsTrue)
	c.Assert(result.Addresses, tc.HasLen, 5)
	c.Assert(result.Addresses[0], tc.DeepEquals, params.Address{
		Value:     "10.0.0.42",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "space1",
	})
	c.Assert(result.Addresses[1], tc.DeepEquals, params.Address{
		Value:     "10.73.37.110",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[2], tc.DeepEquals, params.Address{
		Value:     "10.73.37.111",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[3], tc.DeepEquals, params.Address{
		Value:     "192.168.0.1",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "alpha",
	})
	c.Assert(result.Addresses[4], tc.DeepEquals, params.Address{
		Value:     "1.1.1.42",
		Type:      "ipv4",
		Scope:     "public",
		SpaceName: "alpha",
	})

	machine, err := s.st.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	providerAddrs := machine.ProviderAddresses()

	c.Assert(providerAddrs, tc.DeepEquals, network.SpaceAddresses{
		makeSpaceAddress("10.0.0.42", network.ScopeCloudLocal, "1"),
		makeSpaceAddress("10.73.37.110", network.ScopeCloudLocal, "2"),
		makeSpaceAddress("10.73.37.111", network.ScopeCloudLocal, "2"),
		makeSpaceAddress("192.168.0.1", network.ScopeCloudLocal, network.AlphaSpaceId),
		makeSpaceAddress("1.1.1.42", network.ScopePublic, network.AlphaSpaceId),
	})
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigNoChange(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.expectDefaultSpaces()
	s.machineService.EXPECT().GetMachineUUID(c.Context(), machine.Name("1")).Return("machine-uuid", nil)
	s.networkService.EXPECT().SetProviderNetConfig(c.Context(), machine.UUID("machine-uuid"),
		[]domainnetwork.NetInterface{{
			IsAutoStart: true,
			IsEnabled:   true,
			Addrs: []domainnetwork.NetAddr{
				{
					AddressValue: "10.0.0.42",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				{
					AddressValue: "10.73.37.111",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				{
					AddressValue: "192.168.0.1",
					Scope:        "local-cloud",
					Origin:       "provider",
				},
				// Shadow
				{
					AddressValue: "1.1.1.42",
					Scope:        "public",
					Origin:       "provider",
					IsShadow:     true,
				},
			}},
		}).Return(nil)

	s.st.SetMachineInfo(c, machineInfo{
		id:             "1",
		instanceStatus: statusInfo("foo"),
		providerAddresses: network.SpaceAddresses{
			makeSpaceAddress("10.0.0.42", network.ScopeCloudLocal, "1"),
			makeSpaceAddress("10.73.37.111", network.ScopeCloudLocal, "2"),
			makeSpaceAddress("192.168.0.1", network.ScopeCloudLocal, network.AlphaSpaceId),
			makeSpaceAddress("1.1.1.42", network.ScopePublic, network.AlphaSpaceId),
		},
	})

	results, err := s.api.SetProviderNetworkConfig(c.Context(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						Addresses: []params.Address{
							{
								Value: "10.0.0.42",
								Scope: "local-cloud",
							},
							{
								Value:           "10.73.37.111",
								Scope:           "local-cloud",
								ProviderSpaceID: "my-space-on-maas",
							},
							// This address does not match any of the CIDRs in
							// the known spaces; we expect this to end up in
							// alpha space.
							{
								Value: "192.168.0.1",
								Scope: "local-cloud",
							},
						},
						ShadowAddresses: []params.Address{
							{
								Value: "1.1.1.42",
								Scope: "public",
							},
						},
					},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Modified, tc.IsFalse)
	c.Assert(result.Addresses, tc.HasLen, 4)
	c.Assert(result.Addresses[0], tc.DeepEquals, params.Address{
		Value:     "10.0.0.42",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "space1",
	})
	c.Assert(result.Addresses[1], tc.DeepEquals, params.Address{
		Value:     "10.73.37.111",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "my-space-on-maas",
	})
	c.Assert(result.Addresses[2], tc.DeepEquals, params.Address{
		Value:     "192.168.0.1",
		Type:      "ipv4",
		Scope:     "local-cloud",
		SpaceName: "alpha",
	})
	c.Assert(result.Addresses[3], tc.DeepEquals, params.Address{
		Value:     "1.1.1.42",
		Type:      "ipv4",
		Scope:     "public",
		SpaceName: "alpha",
	})
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigNotAlive(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Dying})
	s.expectDefaultSpaces()

	results, err := s.api.SetProviderNetworkConfig(c.Context(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{{
			Tag: "machine-1",
			Configs: []params.NetworkConfig{{
				Addresses: []params.Address{{Value: "10.0.0.42", Scope: "local-cloud"}},
			}},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.SetProviderNetworkConfigResults{
		Results: []params.SetProviderNetworkConfigResult{{}},
	})

	// We should just return after seeing that the machine is dying.
	s.st.Stub.CheckCallNames(c, "Machine", "Life", "Id")
}

func (s *InstancePollerSuite) expectDefaultSpaces() {
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(
		network.SpaceInfos{
			{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
			{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24"}}},
			{ID: "2", Name: "my-space-on-maas", ProviderId: "my-space-on-maas", Subnets: []network.SubnetInfo{{CIDR: "10.73.37.0/24"}}},
		}, nil)
}

func makeSpaceAddress(ip string, scope network.Scope, spaceID network.SpaceUUID) network.SpaceAddress {
	addr := network.NewSpaceAddress(ip, network.WithScope(scope))
	addr.SpaceID = spaceID
	return addr
}

func statusInfo(st string) status.StatusInfo {
	return status.StatusInfo{Status: status.Status(st)}
}
