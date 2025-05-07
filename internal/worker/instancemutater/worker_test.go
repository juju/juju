// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	apiinstancemutater "github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/instancemutater"
	"github.com/juju/juju/internal/worker/instancemutater/mocks"
	workermocks "github.com/juju/juju/internal/worker/mocks"
	"github.com/juju/juju/rpc/params"
)

type workerConfigSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&workerConfigSuite{})

func (s *workerConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *workerConfigSuite) TestInvalidConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      instancemutater.Config
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      instancemutater.Config{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no Logger",
			config:      instancemutater.Config{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no api",
			config: instancemutater.Config{
				Logger: loggertesting.WrapCheckLog(c),
			},
			err: "nil Facade not valid",
		},
		{
			description: "Test no environ",
			config: instancemutater.Config{
				Logger: loggertesting.WrapCheckLog(c),
				Facade: mocks.NewMockInstanceMutaterAPI(ctrl),
			},
			err: "nil Broker not valid",
		},
		{
			description: "Test no agent",
			config: instancemutater.Config{
				Logger: loggertesting.WrapCheckLog(c),
				Facade: mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker: mocks.NewMockLXDProfiler(ctrl),
			},
			err: "nil AgentConfig not valid",
		},
		{
			description: "Test no tag",
			config: instancemutater.Config{
				Logger:      loggertesting.WrapCheckLog(c),
				Facade:      mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker:      mocks.NewMockLXDProfiler(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
			},
			err: "nil Tag not valid",
		},
		{
			description: "Test no GetMachineWatcher",
			config: instancemutater.Config{
				Logger:      loggertesting.WrapCheckLog(c),
				Facade:      mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker:      mocks.NewMockLXDProfiler(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
				Tag:         names.NewMachineTag("3"),
			},
			err: "nil GetMachineWatcher not valid",
		},
		{
			description: "Test no GetRequiredLXDProfiles",
			config: instancemutater.Config{
				Logger:            loggertesting.WrapCheckLog(c),
				Facade:            mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker:            mocks.NewMockLXDProfiler(ctrl),
				AgentConfig:       mocks.NewMockConfig(ctrl),
				Tag:               names.NewMachineTag("3"),
				GetMachineWatcher: getMachineWatcher,
			},
			err: "nil GetRequiredLXDProfiles not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

var getMachineWatcher = func(context.Context) (watcher.StringsWatcher, error) {
	return &fakeStringsWatcher{}, nil
}

func (s *workerConfigSuite) TestValidConfigValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.Config{
		Facade:                 mocks.NewMockInstanceMutaterAPI(ctrl),
		Logger:                 loggertesting.WrapCheckLog(c),
		Broker:                 mocks.NewMockLXDProfiler(ctrl),
		AgentConfig:            mocks.NewMockConfig(ctrl),
		Tag:                    names.MachineTag{},
		GetMachineWatcher:      getMachineWatcher,
		GetRequiredLXDProfiles: func(_ string) []string { return []string{} },
		GetRequiredContext: func(w instancemutater.MutaterContext) instancemutater.MutaterContext {
			return w
		},
	}
	err := config.Validate()
	c.Assert(err, tc.IsNil)
}

type workerSuite struct {
	testing.IsolationSuite

	logger                 logger.Logger
	facade                 *mocks.MockInstanceMutaterAPI
	broker                 *mocks.MockLXDProfiler
	agentConfig            *mocks.MockConfig
	machine                map[int]*mocks.MockMutaterMachine
	machineTag             names.MachineTag
	machinesWorker         *workermocks.MockWorker
	context                *mocks.MockMutaterContext
	appLXDProfileWorker    map[int]*workermocks.MockWorker
	getRequiredLXDProfiles instancemutater.RequiredLXDProfilesFunc

	// doneWG is a collection of things each test needs to wait to
	// be completed within the test.
	doneWG *sync.WaitGroup

	newWorkerFunc func(instancemutater.Config, instancemutater.RequiredMutaterContextFunc) (worker.Worker, error)
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.logger = loggertesting.WrapCheckLog(c)

	s.newWorkerFunc = instancemutater.NewEnvironTestWorker
	s.machineTag = names.NewMachineTag("0")
	s.getRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default", "juju-testing"}
	}
	s.doneWG = new(sync.WaitGroup)
}

type workerEnvironSuite struct {
	workerSuite
}

var _ = tc.Suite(&workerEnvironSuite{})

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole instance mutator scenario, from start
// to finish for an EnvironWorker.
func (s *workerEnvironSuite) TestFullWorkflow(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectMachineCharmProfilingInfo(0, 3)
	s.expectLXDProfileNamesTrue()
	s.expectSetCharmProfiles(0, 3)
	s.expectAssignLXDProfiles()
	s.expectAliveAndSetModificationStatusIdle(0)
	s.expectModificationStatusApplied(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestVerifyCurrentProfilesTrue(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectAliveAndSetModificationStatusIdle(0)
	s.expectMachineCharmProfilingInfo(0, 2)
	s.expectLXDProfileNamesTrue()
	s.expectSetCharmProfiles(0, 2)
	s.expectModificationStatusApplied(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestRemoveAllCharmProfiles(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectAliveAndSetModificationStatusIdle(0)
	s.expectCharmProfilingInfoRemove(0)
	s.expectLXDProfileNamesTrue()
	s.expectRemoveAllCharmProfiles(0)
	s.expectModificationStatusApplied(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestMachineNotifyTwice(c *tc.C) {
	defer s.setup(c, 2).Finish()

	// A WaitGroup for this test to synchronize when the
	// machine notifications are sent.  The 2nd group must
	// be after machine 0 gets Life() == Alive.
	var group sync.WaitGroup
	s.notifyMachinesWaitGroup([][]string{{"0", "1"}, {"0"}}, &group)
	s.expectFacadeMachineTag(0)
	s.expectFacadeMachineTag(1)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.notifyMachineAppLXDProfile(1, 1)
	s.expectAliveAndSetModificationStatusIdle(1)
	s.expectMachineCharmProfilingInfo(0, 2)
	s.expectMachineCharmProfilingInfo(1, 2)
	s.expectLXDProfileNamesTrue()
	s.expectLXDProfileNamesTrue()
	s.expectSetCharmProfiles(0, 2)
	s.expectSetCharmProfiles(1, 2)
	s.expectMachineAliveStatusIdleMachineDead(0, &group)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestNoChangeFoundOne(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectCharmProfilingInfoSimpleNoChange(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestNoMachineFound(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeReturnsNoMachine()

	err := s.errorKill(c, s.workerForScenario(c))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *workerEnvironSuite) TestCharmProfilingInfoNotProvisioned(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectCharmProfileInfoNotProvisioned(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestCharmProfilingInfoError(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectCharmProfileInfoError(0)
	s.expectContextKillError()

	err := s.errorKill(c, s.workerForScenarioWithContext(c))
	c.Assert(err, jc.Satisfies, params.IsCodeNotSupported)
}

func (s *workerEnvironSuite) TestMachineContainerTypeNotSupported(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerTypeNone()

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestMachineNotSupported(c *tc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.expectContainerType()

	// We need another sync point here, because the worker can be killed
	// before this method is called.
	s.doneWG.Add(1)
	s.machine[0].EXPECT().WatchLXDProfileVerificationNeeded(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.NotifyWatcher, error) {
			s.doneWG.Done()
			return nil, errors.NotSupportedf("")
		},
	)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerSuite) setup(c *tc.C, machineCount int) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockInstanceMutaterAPI(ctrl)
	s.broker = mocks.NewMockLXDProfiler(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.machinesWorker = workermocks.NewMockWorker(ctrl)
	s.context = mocks.NewMockMutaterContext(ctrl)

	s.machine = make(map[int]*mocks.MockMutaterMachine, machineCount)
	s.appLXDProfileWorker = make(map[int]*workermocks.MockWorker)
	for i := 0; i < machineCount; i += 1 {
		s.machine[i] = mocks.NewMockMutaterMachine(ctrl)
		s.appLXDProfileWorker[i] = workermocks.NewMockWorker(ctrl)
	}

	return ctrl
}

// workerForScenario creates worker config based on the suite's mocks.
// Any supplied behaviour functions are executed, then a new worker
// is started successfully and returned.
func (s *workerSuite) workerForScenario(c *tc.C) worker.Worker {
	config := instancemutater.Config{
		Facade:                 s.facade,
		Logger:                 s.logger,
		Broker:                 s.broker,
		AgentConfig:            s.agentConfig,
		Tag:                    s.machineTag,
		GetRequiredLXDProfiles: s.getRequiredLXDProfiles,
	}

	w, err := s.newWorkerFunc(config, func(ctx instancemutater.MutaterContext) instancemutater.MutaterContext {
		return ctx
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) workerForScenarioWithContext(c *tc.C) worker.Worker {
	config := instancemutater.Config{
		Facade:                 s.facade,
		Logger:                 s.logger,
		Broker:                 s.broker,
		AgentConfig:            s.agentConfig,
		Tag:                    s.machineTag,
		GetRequiredLXDProfiles: s.getRequiredLXDProfiles,
	}

	w, err := s.newWorkerFunc(config, func(ctx instancemutater.MutaterContext) instancemutater.MutaterContext {
		c := mutaterContextShim{
			MutaterContext: ctx,
			mockContext:    s.context,
		}
		return c
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) expectFacadeMachineTag(machine int) {
	tag := names.NewMachineTag(strconv.Itoa(machine))
	s.facade.EXPECT().Machine(gomock.Any(), tag).Return(s.machine[machine], nil).AnyTimes()
	s.machine[machine].EXPECT().Tag().Return(tag).AnyTimes()
}

func (s *workerSuite) expectFacadeReturnsNoMachine() {
	do := s.workGroupAddGetDoneWithMachineFunc()
	s.facade.EXPECT().Machine(gomock.Any(), s.machineTag).Return(nil, errors.NewNotFound(nil, "machine")).Do(do)
}

func (s *workerSuite) expectContainerType() {
	for _, m := range s.machine {
		m.EXPECT().ContainerType(gomock.Any()).Return(instance.LXD, nil).AnyTimes()
	}
}

func (s *workerSuite) expectContainerTypeNone() {
	for _, m := range s.machine {
		m.EXPECT().ContainerType(gomock.Any()).Return(instance.NONE, nil).AnyTimes()
	}
}

func (s *workerSuite) expectCharmProfilingInfoSimpleNoChange(machine int) {
	do := s.workGroupAddGetDoneFunc()
	s.machine[machine].EXPECT().CharmProfilingInfo(gomock.Any()).Return(&apiinstancemutater.UnitProfileInfo{}, nil).Do(do)
}

func (s *workerSuite) workGroupAddGetDoneFunc() func(ctx context.Context) (*apiinstancemutater.UnitProfileInfo, error) {
	s.doneWG.Add(1)
	return func(context.Context) (*apiinstancemutater.UnitProfileInfo, error) {
		s.doneWG.Done()
		return nil, nil
	}
}

func (s *workerSuite) workGroupAddGetDoneFuncNoContext() func() {
	s.doneWG.Add(1)
	return func() { s.doneWG.Done() }
}

func (s *workerSuite) workGroupAddGetDoneWithErrorFunc() func(error) {
	s.doneWG.Add(1)
	return func(error) { s.doneWG.Done() }
}

func (s *workerSuite) workGroupAddGetDoneWithMachineFunc() func(ctx context.Context, tag names.MachineTag) (apiinstancemutater.MutaterMachine, error) {
	s.doneWG.Add(1)
	return func(ctx context.Context, tag names.MachineTag) (apiinstancemutater.MutaterMachine, error) {
		s.doneWG.Done()
		return nil, nil
	}
}

func (s *workerSuite) workGroupAddGetDoneWithStatusFunc() func(context.Context, status.Status, string, map[string]interface{}) error {
	s.doneWG.Add(1)
	return func(context.Context, status.Status, string, map[string]interface{}) error {
		s.doneWG.Done()
		return nil
	}
}

func (s *workerSuite) expectLXDProfileNamesTrue() {
	s.broker.EXPECT().LXDProfileNames("juju-23423-0").Return([]string{"default", "juju-testing", "juju-testing-one-2"}, nil)
}

func (s *workerSuite) expectMachineCharmProfilingInfo(machine, rev int) {
	s.expectCharmProfilingInfo(s.machine[machine], rev)
}

func (s *workerSuite) expectCharmProfilingInfo(mock *mocks.MockMutaterMachine, rev int) {
	mock.EXPECT().CharmProfilingInfo(gomock.Any()).Return(&apiinstancemutater.UnitProfileInfo{
		CurrentProfiles: []string{"default", "juju-testing", "juju-testing-one-2"},
		InstanceId:      "juju-23423-0",
		ModelName:       "testing",
		ProfileChanges: []apiinstancemutater.UnitProfileChanges{
			{
				ApplicationName: "one",
				Revision:        rev,
				Profile: lxdprofile.Profile{
					Config: map[string]string{"hi": "bye"},
				},
			},
		},
	}, nil)
}

func (s *workerSuite) expectCharmProfilingInfoRemove(machine int) {
	s.machine[machine].EXPECT().CharmProfilingInfo(gomock.Any()).Return(&apiinstancemutater.UnitProfileInfo{
		CurrentProfiles: []string{"default", "juju-testing", "juju-testing-one-2"},
		InstanceId:      "juju-23423-0",
		ModelName:       "testing",
		ProfileChanges:  []apiinstancemutater.UnitProfileChanges{},
	}, nil)
}

func (s *workerSuite) expectCharmProfileInfoNotProvisioned(machine int) {
	do := s.workGroupAddGetDoneFunc()
	err := params.Error{
		Message: "machine 0 not provisioned",
		Code:    params.CodeNotProvisioned,
	}
	s.machine[machine].EXPECT().CharmProfilingInfo(gomock.Any()).Return(&apiinstancemutater.UnitProfileInfo{}, err).Do(do)
}

func (s *workerSuite) expectCharmProfileInfoError(machine int) {
	do := s.workGroupAddGetDoneFunc()
	err := params.Error{
		Message: "machine 0 not supported",
		Code:    params.CodeNotSupported,
	}
	s.machine[machine].EXPECT().CharmProfilingInfo(gomock.Any()).Return(&apiinstancemutater.UnitProfileInfo{}, err).Do(do)
}

func (s *workerSuite) expectAliveAndSetModificationStatusIdle(machine int) {
	mExp := s.machine[machine].EXPECT()
	mExp.Refresh(gomock.Any()).Return(nil)
	mExp.Life().Return(life.Alive)
	mExp.SetModificationStatus(gomock.Any(), status.Idle, "", nil).Return(nil)
}

func (s *workerSuite) expectMachineAliveStatusIdleMachineDead(machine int, group *sync.WaitGroup) {
	mExp := s.machine[machine].EXPECT()

	group.Add(1)
	notificationSync := func() life.Value { group.Done(); return "" }

	mExp.Refresh(gomock.Any()).Return(nil).Times(2)
	o1 := mExp.Life().Return(life.Alive).Do(notificationSync)

	mExp.SetModificationStatus(gomock.Any(), status.Idle, "", nil).Return(nil)

	s.machine[0].EXPECT().SetModificationStatus(gomock.Any(), status.Applied, "", nil).Return(nil)
	doWithStatus := s.workGroupAddGetDoneWithStatusFunc()
	s.machine[1].EXPECT().SetModificationStatus(gomock.Any(), status.Applied, "", nil).Return(nil).Do(doWithStatus)

	do := s.workGroupAddGetDoneFuncNoContext()
	mExp.Life().Return(life.Dead).After(o1.Call).Do(do)
}

func (s *workerSuite) expectModificationStatusApplied(machine int) {
	do := s.workGroupAddGetDoneWithStatusFunc()
	s.machine[machine].EXPECT().SetModificationStatus(gomock.Any(), status.Applied, "", nil).Return(nil).Do(do)
}

func (s *workerSuite) expectAssignLXDProfiles() {
	profiles := []string{"default", "juju-testing", "juju-testing-one-3"}
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerSuite) expectSetCharmProfiles(machine int, rev int) {
	s.machine[machine].EXPECT().SetCharmProfiles(gomock.Any(), []string{fmt.Sprintf("juju-testing-one-%d", rev)})
}

func (s *workerSuite) expectRemoveAllCharmProfiles(machine int) {
	profiles := []string{"default", "juju-testing"}
	s.machine[machine].EXPECT().SetCharmProfiles(gomock.Any(), []string{})
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

// notifyMachines returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notifyMachines(values [][]string) {
	ch := make(chan []string)
	s.doneWG.Add(1)
	go func() {
		for _, v := range values {
			ch <- v
		}
		s.doneWG.Done()
	}()

	s.machinesWorker.EXPECT().Kill().AnyTimes()
	s.machinesWorker.EXPECT().Wait().Return(nil).AnyTimes()

	s.facade.EXPECT().WatchModelMachines(gomock.Any()).Return(
		&fakeStringsWatcher{
			Worker: s.machinesWorker,
			ch:     ch,
		}, nil)
}

func (s *workerSuite) notifyMachinesWaitGroup(values [][]string, group *sync.WaitGroup) {
	ch := make(chan []string)
	s.doneWG.Add(1)
	go func() {
		for _, v := range values {
			ch <- v
			group.Wait()
		}
		s.doneWG.Done()
	}()

	s.machinesWorker.EXPECT().Kill().AnyTimes()
	s.machinesWorker.EXPECT().Wait().Return(nil).AnyTimes()

	s.facade.EXPECT().WatchModelMachines(gomock.Any()).Return(
		&fakeStringsWatcher{
			Worker: s.machinesWorker,
			ch:     ch,
		}, nil)
}

// notifyAppLXDProfile returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notifyMachineAppLXDProfile(machine, times int) {
	s.notifyAppLXDProfile(s.machine[machine], machine, times)
}

func (s *workerContainerSuite) notifyContainerAppLXDProfile(times int) {
	s.notifyAppLXDProfile(s.lxdContainer, 0, times)
}

func (s *workerSuite) notifyAppLXDProfile(mock *mocks.MockMutaterMachine, which, times int) {
	ch := make(chan struct{})
	s.doneWG.Add(1)
	go func() {
		for i := 0; i < times; i += 1 {
			ch <- struct{}{}
		}
		s.doneWG.Done()
	}()

	w := s.appLXDProfileWorker[which]
	w.EXPECT().Kill().AnyTimes()
	w.EXPECT().Wait().Return(nil).AnyTimes()

	mock.EXPECT().WatchLXDProfileVerificationNeeded(gomock.Any()).Return(
		&fakeNotifyWatcher{
			Worker: w,
			ch:     ch,
		}, nil)
}

// mutaterContextShim is required to override the KillWithError context. We
// can't mock out the whole thing as their are private methods, so we just
// compose it and send it back with a new KillWithError method.
type mutaterContextShim struct {
	instancemutater.MutaterContext
	mockContext *mocks.MockMutaterContext
}

func (c mutaterContextShim) KillWithError(err error) {
	if c.mockContext != nil {
		c.mockContext.KillWithError(err)
	}
	// We still want to call the original context to ensure that errorKill
	// still passes.
	c.MutaterContext.KillWithError(err)
}

func (s *workerSuite) expectContextKillError() {
	do := s.workGroupAddGetDoneWithErrorFunc()
	s.context.EXPECT().KillWithError(gomock.Any()).Do(do)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *workerSuite) cleanKill(c *tc.C, w worker.Worker) {
	s.waitDone(c)
	workertest.CleanKill(c, w)
}

// errorKill waits for notifications to be processed, then waits for the input
// worker to be killed.  Any error is returned to the caller. If either ops
// time out, the test fails.
func (s *workerSuite) errorKill(c *tc.C, w worker.Worker) error {
	s.waitDone(c)
	return workertest.CheckKill(c, w)
}

func (s *workerSuite) waitDone(c *tc.C) {
	ch := make(chan struct{})
	go func() {
		s.doneWG.Wait()
		close(ch)
	}()

	select {
	case <-ch:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
}

type workerContainerSuite struct {
	workerSuite

	lxdContainerTag names.Tag
	lxdContainer    *mocks.MockMutaterMachine
}

var _ = tc.Suite(&workerContainerSuite{})

func (s *workerContainerSuite) SetUpTest(c *tc.C) {
	s.workerSuite.SetUpTest(c)

	s.lxdContainerTag = names.NewMachineTag("0/lxd/0")
	s.newWorkerFunc = instancemutater.NewContainerTestWorker
	s.getRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default"}
	}
}

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole instance mutator scenario, from start
// to finish for a ContainerWorker.
func (s *workerContainerSuite) TestFullWorkflow(c *tc.C) {
	defer s.setup(c).Finish()

	s.notifyContainers(0, [][]string{{"0/lxd/0"}})
	s.expectFacadeMachineTag(0)
	s.expectFacadeContainerTags()
	s.expectContainerTypes()
	s.notifyContainerAppLXDProfile(1)
	s.expectContainerCharmProfilingInfo(3)
	s.expectLXDProfileNamesTrue()
	s.expectContainerSetCharmProfiles()
	s.expectAssignLXDProfiles()
	s.expectContainerAliveAndSetModificationStatusIdle()
	s.expectContainerModificationStatusApplied()

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerContainerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := s.workerSuite.setup(c, 1)
	s.lxdContainer = mocks.NewMockMutaterMachine(ctrl)
	return ctrl
}

func (s *workerContainerSuite) expectFacadeContainerTags() {
	s.facade.EXPECT().Machine(gomock.Any(), s.lxdContainerTag).Return(s.lxdContainer, nil).AnyTimes()
	s.lxdContainer.EXPECT().Tag().Return(s.lxdContainerTag.(names.MachineTag)).AnyTimes()
}

func (s *workerContainerSuite) expectContainerTypes() {
	s.lxdContainer.EXPECT().ContainerType(gomock.Any()).Return(instance.LXD, nil).AnyTimes()
}

func (s *workerContainerSuite) expectContainerCharmProfilingInfo(rev int) {
	s.expectCharmProfilingInfo(s.lxdContainer, rev)
}

func (s *workerContainerSuite) expectContainerAliveAndSetModificationStatusIdle() {
	cExp := s.lxdContainer.EXPECT()
	cExp.Refresh(gomock.Any()).Return(nil)
	cExp.Life().Return(life.Alive)
	cExp.SetModificationStatus(gomock.Any(), status.Idle, gomock.Any(), gomock.Any()).Return(nil)
}

func (s *workerContainerSuite) expectContainerModificationStatusApplied() {
	do := s.workGroupAddGetDoneWithStatusFunc()
	s.lxdContainer.EXPECT().SetModificationStatus(gomock.Any(), status.Applied, "", nil).Return(nil).Do(do)
}

func (s *workerContainerSuite) expectAssignLXDProfiles() {
	profiles := []string{"default", "juju-testing-one-3"}
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerContainerSuite) expectContainerSetCharmProfiles() {
	s.lxdContainer.EXPECT().SetCharmProfiles(gomock.Any(), []string{"juju-testing-one-3"})
}

// notifyContainers returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerContainerSuite) notifyContainers(machine int, values [][]string) {
	ch := make(chan []string)
	s.doneWG.Add(1)
	go func() {
		for _, v := range values {
			ch <- v
		}
		s.doneWG.Done()
	}()

	s.machinesWorker.EXPECT().Kill().AnyTimes()
	s.machinesWorker.EXPECT().Wait().Return(nil).AnyTimes()

	s.machine[machine].EXPECT().WatchContainers(gomock.Any()).Return(
		&fakeStringsWatcher{
			Worker: s.machinesWorker,
			ch:     ch,
		}, nil)
}

type fakeStringsWatcher struct {
	worker.Worker
	ch <-chan []string
}

func (w *fakeStringsWatcher) Changes() watcher.StringsChannel {
	return w.ch
}

type fakeNotifyWatcher struct {
	worker.Worker
	ch <-chan struct{}
}

func (w *fakeNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.ch
}
