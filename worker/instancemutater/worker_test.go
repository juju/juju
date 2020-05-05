// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"strconv"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	apiinstancemutater "github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
	workermocks "github.com/juju/juju/worker/mocks"
)

type workerConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workerConfigSuite{})

func (s *workerConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *workerConfigSuite) TestInvalidConfigValidate(c *gc.C) {
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
				Logger: loggo.GetLogger("test"),
			},
			err: "nil Facade not valid",
		},
		{
			description: "Test no environ",
			config: instancemutater.Config{
				Logger: loggo.GetLogger("test"),
				Facade: mocks.NewMockInstanceMutaterAPI(ctrl),
			},
			err: "nil Broker not valid",
		},
		{
			description: "Test no agent",
			config: instancemutater.Config{
				Logger: loggo.GetLogger("test"),
				Facade: mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker: mocks.NewMockLXDProfiler(ctrl),
			},
			err: "nil AgentConfig not valid",
		},
		{
			description: "Test no tag",
			config: instancemutater.Config{
				Logger:      loggo.GetLogger("test"),
				Facade:      mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker:      mocks.NewMockLXDProfiler(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
			},
			err: "nil Tag not valid",
		},
		{
			description: "Test no GetMachineWatcher",
			config: instancemutater.Config{
				Logger:      loggo.GetLogger("test"),
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
				Logger:            loggo.GetLogger("test"),
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
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

var getMachineWatcher = func() (watcher.StringsWatcher, error) {
	return &fakeStringsWatcher{}, nil
}

func (s *workerConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.Config{
		Facade:                 mocks.NewMockInstanceMutaterAPI(ctrl),
		Logger:                 loggo.GetLogger("test"),
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
	c.Assert(err, gc.IsNil)
}

type workerSuite struct {
	testing.IsolationSuite

	logger                 loggo.Logger
	facade                 *mocks.MockInstanceMutaterAPI
	broker                 *mocks.MockLXDProfiler
	agentConfig            *mocks.MockConfig
	machine                map[int]*mocks.MockMutaterMachine
	machineTag             names.Tag
	machinesWorker         *workermocks.MockWorker
	context                *mocks.MockMutaterContext
	appLXDProfileWorker    map[int]*workermocks.MockWorker
	getRequiredLXDProfiles instancemutater.RequiredLXDProfilesFunc

	// doneWG is a collection of things each test needs to wait to
	// be completed within the test.
	doneWG sync.WaitGroup

	newWorkerFunc func(instancemutater.Config, instancemutater.RequiredMutaterContextFunc) (worker.Worker, error)
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.logger = loggo.GetLogger("workerSuite")
	s.logger.SetLogLevel(loggo.TRACE)

	s.newWorkerFunc = instancemutater.NewEnvironTestWorker
	s.machineTag = names.NewMachineTag("0")
	s.getRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default", "juju-testing"}
	}
}

type workerEnvironSuite struct {
	workerSuite
}

var _ = gc.Suite(&workerEnvironSuite{})

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole instance mutator scenario, from start
// to finish for an EnvironWorker.
func (s *workerEnvironSuite) TestFullWorkflow(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectMachineCharmProfilingInfo(0, 3)
	s.expectLXDProfileNamesTrue()
	s.expectSetCharmProfiles(0)
	s.expectAssignLXDProfiles()
	s.expectAliveAndSetModificationStatusIdle(0)
	s.expectModificationStatusApplied(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestVerifyCurrentProfilesTrue(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectAliveAndSetModificationStatusIdle(0)
	s.expectMachineCharmProfilingInfo(0, 2)
	s.expectLXDProfileNamesTrue()
	s.expectModificationStatusApplied(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestRemoveAllCharmProfiles(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectAliveAndSetModificationStatusIdle(0)
	s.expectCharmProfilingInfoRemove(0)
	s.expectLXDProfileNamesTrue()
	s.expectRemoveAllCharmProfiles(0)
	s.expectModificationStatusApplied(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestMachineNotifyTwice(c *gc.C) {
	defer s.setup(c, 2).Finish()

	// A WaitGroup for this test to synchronize when the
	// machine notifications are sent.  The 2nd group must
	// be after machine 0 gets Life() == Alive.
	var group sync.WaitGroup
	s.notifyMachinesWaitGroup([][]string{{"0", "1"}, {"0"}}, &group)
	s.expectFacadeMachineTag(0)
	s.expectFacadeMachineTag(1)
	s.notifyMachineAppLXDProfile(0, 1)
	s.notifyMachineAppLXDProfile(1, 1)
	s.expectAliveAndSetModificationStatusIdle(1)
	s.expectMachineCharmProfilingInfo(0, 2)
	s.expectMachineCharmProfilingInfo(1, 2)
	s.expectLXDProfileNamesTrue()
	s.expectLXDProfileNamesTrue()
	s.expectMachineAliveStatusIdleMachineDead(0, &group)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestNoChangeFoundOne(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectCharmProfilingInfoSimpleNoChange(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestNoMachineFound(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeReturnsNoMachine()

	err := s.errorKill(c, s.workerForScenario(c))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *workerEnvironSuite) TestCharmProfilingInfoNotProvisioned(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectCharmProfileInfoNotProvisioned(0)

	s.cleanKill(c, s.workerForScenario(c))
}

func (s *workerEnvironSuite) TestCharmProfilingInfoError(c *gc.C) {
	defer s.setup(c, 1).Finish()

	s.notifyMachines([][]string{{"0"}})
	s.expectFacadeMachineTag(0)
	s.notifyMachineAppLXDProfile(0, 1)
	s.expectCharmProfileInfoError(0)
	s.expectContextKillError()

	err := s.errorKill(c, s.workerForScenarioWithContext(c))
	c.Assert(err, jc.Satisfies, params.IsCodeNotSupported)
}

func (s *workerSuite) setup(c *gc.C, machineCount int) *gomock.Controller {
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

	s.expectContainerTypeNone()
	return ctrl
}

// workerForScenario creates worker config based on the suite's mocks.
// Any supplied behaviour functions are executed, then a new worker
// is started successfully and returned.
func (s *workerSuite) workerForScenario(c *gc.C) worker.Worker {
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

func (s *workerSuite) workerForScenarioWithContext(c *gc.C) worker.Worker {
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
	s.facade.EXPECT().Machine(tag).Return(s.machine[machine], nil).AnyTimes()
	s.machine[machine].EXPECT().Tag().Return(tag).AnyTimes()
}

func (s *workerSuite) expectFacadeReturnsNoMachine() {
	do := s.workGroupAddGetDoneFunc()
	s.facade.EXPECT().Machine(s.machineTag).Return(nil, errors.NewNotFound(nil, "machine")).Do(do)
}

func (s *workerSuite) expectContainerTypeNone() {
	for _, m := range s.machine {
		m.EXPECT().ContainerType().Return(instance.NONE, nil).AnyTimes()
	}
}

func (s *workerSuite) expectCharmProfilingInfoSimpleNoChange(machine int) {
	do := s.workGroupAddGetDoneFunc()
	s.machine[machine].EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, nil).Do(do)
}

func (s *workerSuite) workGroupAddGetDoneFunc() func(_ ...interface{}) {
	s.doneWG.Add(1)
	return func(_ ...interface{}) { s.doneWG.Done() }
}

func (s *workerSuite) expectLXDProfileNamesTrue() {
	s.broker.EXPECT().LXDProfileNames("juju-23423-0").Return([]string{"default", "juju-testing", "juju-testing-one-2"}, nil)
}

func (s *workerSuite) expectMachineCharmProfilingInfo(machine, rev int) {
	s.expectCharmProfilingInfo(s.machine[machine], rev)
}

func (s *workerSuite) expectCharmProfilingInfo(mock *mocks.MockMutaterMachine, rev int) {
	mock.EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{
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
	s.machine[machine].EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{
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
	s.machine[machine].EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, err).Do(do)
}

func (s *workerSuite) expectCharmProfileInfoError(machine int) {
	do := s.workGroupAddGetDoneFunc()
	err := params.Error{
		Message: "machine 0 not supported",
		Code:    params.CodeNotSupported,
	}
	s.machine[machine].EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, err).Do(do)
}

func (s *workerSuite) expectAliveAndSetModificationStatusIdle(machine int) {
	mExp := s.machine[machine].EXPECT()
	mExp.Refresh().Return(nil)
	mExp.Life().Return(life.Alive)
	mExp.SetModificationStatus(status.Idle, "", nil).Return(nil)
}

func (s *workerSuite) expectMachineAliveStatusIdleMachineDead(machine int, group *sync.WaitGroup) {
	s.doneWG.Add(1)
	do := s.workGroupAddGetDoneFunc()

	mExp := s.machine[machine].EXPECT()

	group.Add(1)
	notificationSync := func(_ ...interface{}) { group.Done() }

	mExp.Refresh().Return(nil).Times(2)
	o1 := mExp.Life().Return(life.Alive).Do(notificationSync)

	mExp.SetModificationStatus(status.Idle, "", nil).Return(nil)

	s.machine[0].EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil)
	s.machine[1].EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil).Do(do)

	mExp.Life().Return(life.Dead).After(o1).Do(do)
}

func (s *workerSuite) expectModificationStatusApplied(machine int) {
	do := s.workGroupAddGetDoneFunc()
	s.machine[machine].EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil).Do(do)
}

func (s *workerSuite) expectAssignLXDProfiles() {
	profiles := []string{"default", "juju-testing", "juju-testing-one-3"}
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerSuite) expectSetCharmProfiles(machine int) {
	s.machine[machine].EXPECT().SetCharmProfiles([]string{"default", "juju-testing", "juju-testing-one-3"})
}

func (s *workerSuite) expectRemoveAllCharmProfiles(machine int) {
	profiles := []string{"default", "juju-testing"}
	s.machine[machine].EXPECT().SetCharmProfiles(profiles)
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

	s.facade.EXPECT().WatchMachines().Return(
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

	s.facade.EXPECT().WatchMachines().Return(
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

	mock.EXPECT().WatchLXDProfileVerificationNeeded().Return(
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
	do := s.workGroupAddGetDoneFunc()
	s.context.EXPECT().KillWithError(gomock.Any()).Do(do)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *workerSuite) cleanKill(c *gc.C, w worker.Worker) {
	s.waitDone(c)
	workertest.CleanKill(c, w)
}

// errorKill waits for notifications to be processed, then waits for the input
// worker to be killed.  Any error is returned to the caller. If either ops
// time out, the test fails.
func (s *workerSuite) errorKill(c *gc.C, w worker.Worker) error {
	s.waitDone(c)
	return workertest.CheckKill(c, w)
}

func (s *workerSuite) waitDone(c *gc.C) {
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
	kvmContainerTag names.Tag
	lxdContainer    *mocks.MockMutaterMachine
	kvmContainer    *mocks.MockMutaterMachine
}

var _ = gc.Suite(&workerContainerSuite{})

func (s *workerContainerSuite) SetUpTest(c *gc.C) {
	s.workerSuite.SetUpTest(c)

	s.lxdContainerTag = names.NewMachineTag("0/lxd/0")
	s.kvmContainerTag = names.NewMachineTag("0/kvm/0")
	s.newWorkerFunc = instancemutater.NewContainerTestWorker
	s.getRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default"}
	}
}

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole instance mutator scenario, from start
// to finish for a ContainerWorker.
func (s *workerContainerSuite) TestFullWorkflow(c *gc.C) {
	defer s.setup(c).Finish()

	s.notifyContainers(0, [][]string{{"0/lxd/0", "0/kvm/0"}})
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

func (s *workerContainerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.workerSuite.setup(c, 1)
	s.lxdContainer = mocks.NewMockMutaterMachine(ctrl)
	s.kvmContainer = mocks.NewMockMutaterMachine(ctrl)
	return ctrl
}

func (s *workerContainerSuite) expectFacadeContainerTags() {
	s.facade.EXPECT().Machine(s.lxdContainerTag).Return(s.lxdContainer, nil).AnyTimes()
	s.lxdContainer.EXPECT().Tag().Return(s.lxdContainerTag).AnyTimes()
	s.facade.EXPECT().Machine(s.kvmContainerTag).Return(s.kvmContainer, nil).AnyTimes()
	s.kvmContainer.EXPECT().Tag().Return(s.kvmContainerTag).AnyTimes()
}

func (s *workerContainerSuite) expectContainerTypes() {
	s.lxdContainer.EXPECT().ContainerType().Return(instance.LXD, nil).AnyTimes()
	s.kvmContainer.EXPECT().ContainerType().Return(instance.KVM, nil).AnyTimes()
}

func (s *workerContainerSuite) expectContainerCharmProfilingInfo(rev int) {
	s.expectCharmProfilingInfo(s.lxdContainer, rev)
}

func (s *workerContainerSuite) expectContainerAliveAndSetModificationStatusIdle() {
	cExp := s.lxdContainer.EXPECT()
	cExp.Refresh().Return(nil)
	cExp.Life().Return(life.Alive)
	cExp.SetModificationStatus(status.Idle, gomock.Any(), gomock.Any()).Return(nil)
}

func (s *workerContainerSuite) expectContainerModificationStatusApplied() {
	do := s.workGroupAddGetDoneFunc()
	s.lxdContainer.EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil).Do(do)
}

func (s *workerContainerSuite) expectAssignLXDProfiles() {
	profiles := []string{"default", "juju-testing-one-3"}
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerContainerSuite) expectContainerSetCharmProfiles() {
	s.lxdContainer.EXPECT().SetCharmProfiles([]string{"default", "juju-testing-one-3"})
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

	s.machine[machine].EXPECT().WatchContainers().Return(
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
