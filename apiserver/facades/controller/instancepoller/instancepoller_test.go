// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	"github.com/juju/juju/apiserver/facades/controller/instancepoller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	machineerrors "github.com/juju/juju/domain/machine/errors"
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

	machineEntities     params.Entities
	machineErrorResults params.ErrorResults

	mixedEntities     params.Entities
	mixedErrorResults params.ErrorResults

	clock clock.Clock
}

var _ = tc.Suite(&InstancePollerSuite{})

func (s *InstancePollerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	return ctrl
}

func (s *InstancePollerSuite) setupAPI(c *tc.C) (err error) {
	s.api, err = instancepoller.NewInstancePollerAPI(
		nil,
		s.networkService,
		s.machineService,
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

func (s *InstancePollerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Updating a machine-sourced address should change its origin to "provider".
`)
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

	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Alive})
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dying})

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

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "Life")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "Life")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestLifeFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1"); Life not called
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.Life() - unused
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", life: state.Alive})
	s.st.SetMachineInfo(c, machineInfo{id: "2", life: state.Dead})
	s.st.SetMachineInfo(c, machineInfo{id: "3", life: state.Dying})

	result, err := s.api.Life(c.Context(), s.machineEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Life: life.Dead},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "Life")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestInstanceIdSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("uuid-1")).Return("i-foo", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("uuid-2", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("uuid-2")).Return("", nil)
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
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("uuid-2")).Return("i-2", errors.New("FAIL"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("3")).Return("uuid-3", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("uuid-3")).Return("", machineerrors.NotProvisioned)
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

	now := time.Now()
	s1 := status.StatusInfo{
		Status:  status.Error,
		Message: "not really",
		Data: map[string]interface{}{
			"price": 4.2,
			"bool":  false,
			"bar":   []string{"a", "b"},
		},
		Since: &now,
	}
	s2 := status.StatusInfo{}
	s.st.SetMachineInfo(c, machineInfo{id: "1", status: s1})
	s.st.SetMachineInfo(c, machineInfo{id: "2", status: s2})

	result, err := s.api.Status(c.Context(), s.mixedEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{
				Status: status.Error.String(),
				Info:   s1.Message,
				Data:   s1.Data,
				Since:  s1.Since,
			},
			{Status: "", Info: "", Data: nil, Since: nil},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckCall(c, 1, "Status")
	s.st.CheckFindEntityCall(c, 2, "2")
	s.st.CheckCall(c, 3, "Status")
	s.st.CheckFindEntityCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestStatusFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1"); Status not called
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.Status()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1"})
	s.st.SetMachineInfo(c, machineInfo{id: "2"})

	result, err := s.api.Status(c.Context(), s.machineEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckFindEntityCall(c, 0, "1")
	s.st.CheckFindEntityCall(c, 1, "2")
	s.st.CheckCall(c, 2, "Status")
	s.st.CheckFindEntityCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestInstanceStatusSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.InstanceStatus(c.Context(), s.mixedEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Status: "foo"},
			{Status: ""},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		},
	},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "InstanceStatus")
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "InstanceStatus")
	s.st.CheckMachineCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestInstanceStatusFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.InstanceStatus()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.InstanceStatus(c.Context(), s.machineEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	s.st.CheckCall(c, 2, "InstanceStatus")
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetInstanceStatusSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: ""},
			{Tag: "machine-2", Status: "new status"},
			{Tag: "machine-42", Status: ""},
			{Tag: "application-unknown", Status: ""},
			{Tag: "invalid-tag", Status: ""},
			{Tag: "unit-missing-1", Status: ""},
			{Tag: "", Status: ""},
			{Tag: "42", Status: ""},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, s.mixedErrorResults)

	now := s.clock.Now()
	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "SetInstanceStatus", status.StatusInfo{Status: "", Since: &now})
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "SetInstanceStatus", status.StatusInfo{Status: "new status", Since: &now})
	s.st.CheckMachineCall(c, 4, "42")

	// Ensure machines were updated.
	machine, err := s.st.Machine("1")
	c.Assert(err, tc.ErrorIsNil)
	// TODO (perrito666) there should not be an empty StatusInfo here,
	// this is certainly a smell.
	setStatus, err := machine.InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)
	setStatus.Since = nil
	c.Assert(setStatus, tc.DeepEquals, status.StatusInfo{})

	machine, err = s.st.Machine("2")
	c.Assert(err, tc.ErrorIsNil)
	setStatus, err = machine.InstanceStatus()
	c.Assert(err, tc.ErrorIsNil)
	setStatus.Since = nil
	c.Assert(setStatus, tc.DeepEquals, status.StatusInfo{Status: "new status"})
}

func (s *InstancePollerSuite) TestSetInstanceStatusFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.SetInstanceStatus()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})
	s.st.SetMachineInfo(c, machineInfo{id: "2", instanceStatus: statusInfo("")})

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-1", Status: "new"},
			{Tag: "machine-2", Status: "invalid"},
			{Tag: "machine-3", Status: ""},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, s.machineErrorResults)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	now := s.clock.Now()
	s.st.CheckCall(c, 2, "SetInstanceStatus", status.StatusInfo{Status: "invalid", Since: &now})
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestAreManuallyProvisionedSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetMachineInfo(c, machineInfo{id: "1", isManual: true})
	s.st.SetMachineInfo(c, machineInfo{id: "2", isManual: false})

	result, err := s.api.AreManuallyProvisioned(c.Context(), s.mixedEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: true},
			{Result: false},
			{Error: apiservertesting.NotFoundError("machine 42")},
			{Error: apiservertesting.ServerError(`"application-unknown" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"invalid-tag" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"unit-missing-1" is not a valid machine tag`)},
			{Error: apiservertesting.ServerError(`"" is not a valid tag`)},
			{Error: apiservertesting.ServerError(`"42" is not a valid tag`)},
		}},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckCall(c, 1, "IsManual")
	s.st.CheckMachineCall(c, 2, "2")
	s.st.CheckCall(c, 3, "IsManual")
	s.st.CheckMachineCall(c, 4, "42")
}

func (s *InstancePollerSuite) TestAreManuallyProvisionedFailure(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetErrors(
		errors.New("pow!"),                   // m1 := FindEntity("1")
		nil,                                  // m2 := FindEntity("2")
		errors.New("FAIL"),                   // m2.IsManual()
		errors.NotProvisionedf("machine 42"), // FindEntity("3") (ensure wrapping is preserved)
	)
	s.st.SetMachineInfo(c, machineInfo{id: "1", isManual: true})
	s.st.SetMachineInfo(c, machineInfo{id: "2", isManual: false})

	result, err := s.api.AreManuallyProvisioned(c.Context(), s.machineEntities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ServerError("pow!")},
			{Error: apiservertesting.ServerError("FAIL")},
			{Error: apiservertesting.NotProvisionedError("42")},
		}},
	)

	s.st.CheckMachineCall(c, 0, "1")
	s.st.CheckMachineCall(c, 1, "2")
	s.st.CheckCall(c, 2, "IsManual")
	s.st.CheckMachineCall(c, 3, "3")
}

func (s *InstancePollerSuite) TestSetProviderNetworkConfigSuccess(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.expectDefaultSpaces()

	s.st.SetMachineInfo(c, machineInfo{id: "1", instanceStatus: statusInfo("foo")})

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujutesting.FakeControllerConfig(), nil)

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

func (s *InstancePollerSuite) TestSetProviderNetworkConfigRelinquishUnseen(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.expectDefaultSpaces()

	// Hardware address not matched.
	dev := mocks.NewMockLinkLayerDevice(ctrl)
	dExp := dev.EXPECT()
	dExp.MACAddress().Return("01:01:01:01:01:01").MinTimes(1)
	dExp.Name().Return("eth0").MinTimes(1)
	dExp.SetProviderIDOps(network.Id("")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	// Address should be set back to machine origin.
	addr := mocks.NewMockLinkLayerAddress(ctrl)
	addr.EXPECT().DeviceName().Return("eth0")
	addr.EXPECT().SetOriginOps(network.OriginMachine).Return([]txn.Op{{C: "address-origin-manual"}})

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{dev},
		addresses:        []networkingcommon.LinkLayerAddress{addr},
	})

	result, err := s.api.SetProviderNetworkConfig(c.Context(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag:     "machine-1",
				Configs: []params.NetworkConfig{{MACAddress: "00:00:00:00:00:00"}},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)

	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
			c.Check(call.Args, tc.DeepEquals, []interface{}{[]txn.Op{
				{C: "machine-alive"},
				{C: "dev-provider-id"},
				{C: "address-origin-manual"},
			}})
		}
	}
	c.Assert(buildCalled, tc.IsTrue)
}

func (s *InstancePollerSuite) TestSetProviderNetworkProviderIDGoesToEthernetDev(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.expectDefaultSpaces()

	// Ethernet device will have the provider ID set.
	ethDev := mocks.NewMockLinkLayerDevice(ctrl)
	ethExp := ethDev.EXPECT()
	ethExp.MACAddress().Return("00:00:00:00:00:00").MinTimes(1)
	ethExp.Name().Return("eth0").MinTimes(1)
	ethExp.ProviderID().Return(network.Id("")).MinTimes(1)
	ethExp.SetProviderIDOps(network.Id("p-dev")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	// Bridge has the same MAC, but will not get the provider ID.
	brDev := mocks.NewMockLinkLayerDevice(ctrl)
	brExp := brDev.EXPECT()
	brExp.MACAddress().Return("00:00:00:00:00:00").MinTimes(1)
	brExp.Name().Return("br-eth0").AnyTimes()
	brExp.Type().Return(network.BridgeDevice)

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{ethDev, brDev},
		addresses:        []networkingcommon.LinkLayerAddress{},
	})

	result, err := s.api.SetProviderNetworkConfig(c.Context(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						// This should still be matched based on hardware address.
						InterfaceName: "",
						MACAddress:    "00:00:00:00:00:00",
						ProviderId:    "p-dev",
					},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)

	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
		}
	}
	c.Assert(buildCalled, tc.IsTrue)
}

func (s *InstancePollerSuite) TestSetProviderNetworkProviderIDMultipleRefsError(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()
	err := s.setupAPI(c)
	c.Assert(err, tc.ErrorIsNil)

	s.expectDefaultSpaces()

	ethDev := mocks.NewMockLinkLayerDevice(ctrl)
	ethExp := ethDev.EXPECT()
	ethExp.Name().Return("eth0").MinTimes(1)
	ethExp.ProviderID().Return(network.Id("")).MinTimes(1)
	ethExp.SetProviderIDOps(network.Id("p-dev")).Return([]txn.Op{{C: "dev-provider-id"}}, nil)

	brDev := mocks.NewMockLinkLayerDevice(ctrl)
	brExp := brDev.EXPECT()
	brExp.Name().Return("br-eth0").AnyTimes()
	brExp.ProviderID().Return(network.Id("")).MinTimes(1)
	// Note no calls to SetProviderIDOps.

	s.st.SetMachineInfo(c, machineInfo{
		id:               "1",
		instanceStatus:   statusInfo("foo"),
		linkLayerDevices: []networkingcommon.LinkLayerDevice{ethDev, brDev},
		addresses:        []networkingcommon.LinkLayerAddress{},
	})

	// Same provider ID for both.
	result, err := s.api.SetProviderNetworkConfig(c.Context(), params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{
			{
				Tag: "machine-1",
				Configs: []params.NetworkConfig{
					{
						InterfaceName: "eth0",
						MACAddress:    "aa:00:00:00:00:00",
						ProviderId:    "p-dev",
					},
					{
						InterfaceName: "br-eth0",
						MACAddress:    "bb:00:00:00:00:00",
						ProviderId:    "p-dev",
					},
				},
			},
		},
	})

	// The error is logged but not returned.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)

	// But we should not have registered a successful call to Build.
	// This returns an error.
	var buildCalled bool
	for _, call := range s.st.Calls() {
		if call.FuncName == "ApplyOperation.Build" {
			buildCalled = true
		}
	}
	c.Assert(buildCalled, tc.IsFalse)
}

func (s *InstancePollerSuite) expectDefaultSpaces() {
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(
		network.SpaceInfos{
			{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
			{ID: "1", Name: "space1", Subnets: []network.SubnetInfo{{CIDR: "10.0.0.0/24"}}},
			{ID: "2", Name: "my-space-on-maas", ProviderId: "my-space-on-maas", Subnets: []network.SubnetInfo{{CIDR: "10.73.37.0/24"}}},
		}, nil)
}

func makeSpaceAddress(ip string, scope network.Scope, spaceID string) network.SpaceAddress {
	addr := network.NewSpaceAddress(ip, network.WithScope(scope))
	addr.SpaceID = spaceID
	return addr
}

func statusInfo(st string) status.StatusInfo {
	return status.StatusInfo{Status: status.Status(st)}
}
