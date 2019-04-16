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
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	apiinstancemutater "github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/apiserver/params"
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
				Logger: mocks.NewMockLogger(ctrl),
			},
			err: "nil Facade not valid",
		},
		{
			description: "Test no environ",
			config: instancemutater.Config{
				Logger: mocks.NewMockLogger(ctrl),
				Facade: mocks.NewMockInstanceMutaterAPI(ctrl),
			},
			err: "nil Broker not valid",
		},
		{
			description: "Test no agent",
			config: instancemutater.Config{
				Logger: mocks.NewMockLogger(ctrl),
				Facade: mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker: mocks.NewMockLXDProfiler(ctrl),
			},
			err: "nil AgentConfig not valid",
		},
		{
			description: "Test no tag",
			config: instancemutater.Config{
				Logger:      mocks.NewMockLogger(ctrl),
				Facade:      mocks.NewMockInstanceMutaterAPI(ctrl),
				Broker:      mocks.NewMockLXDProfiler(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
			},
			err: "nil Tag not valid",
		},
		{
			description: "Test no GetMachineWatcher",
			config: instancemutater.Config{
				Logger:      mocks.NewMockLogger(ctrl),
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
				Logger:            mocks.NewMockLogger(ctrl),
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
		Logger:                 mocks.NewMockLogger(ctrl),
		Broker:                 mocks.NewMockLXDProfiler(ctrl),
		AgentConfig:            mocks.NewMockConfig(ctrl),
		Tag:                    names.MachineTag{},
		GetMachineWatcher:      getMachineWatcher,
		GetRequiredLXDProfiles: func(_ string) []string { return []string{} },
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}

type workerSuite struct {
	loggerSuite

	facade                 *mocks.MockInstanceMutaterAPI
	broker                 *mocks.MockLXDProfiler
	agentConfig            *mocks.MockConfig
	machine                map[int]*mocks.MockMutaterMachine
	machineTag             names.Tag
	machinesWorker         *workermocks.MockWorker
	appLXDProfileWorker    map[int]*workermocks.MockWorker
	getRequiredLXDProfiles instancemutater.RequiredLXDProfilesFunc

	// doneWG is a collection of things each test needs to wait to
	// be completed within the test.
	doneWG sync.WaitGroup

	newWorkerFunc func(instancemutater.Config) (worker.Worker, error)
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.newWorkerFunc = instancemutater.NewEnvironWorker
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

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{{"0"}}),
		s.expectFacadeMachineTag(0),
		s.notifyMachineAppLXDProfile(0, 1),
		s.expectMachineCharmProfilingInfo(0, 3),
		s.expectLXDProfileNamesTrue,
		s.expectSetCharmProfiles(0),
		s.expectAssignLXDProfiles,
		s.expectAliveAndSetModificationStatusIdle(0),
		s.expectModificationStatusApplied(0),
	)
	s.cleanKill(c, w)
}

func (s *workerEnvironSuite) TestVerifyCurrentProfilesTrue(c *gc.C) {
	defer s.setup(c, 1).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{{"0"}}),
		s.expectFacadeMachineTag(0),
		s.notifyMachineAppLXDProfile(0, 1),
		s.expectAliveAndSetModificationStatusIdle(0),
		s.expectMachineCharmProfilingInfo(0, 2),
		s.expectLXDProfileNamesTrue,
		s.expectModificationStatusApplied(0),
	)
	s.cleanKill(c, w)
}

func (s *workerEnvironSuite) TestMachineNotifyTwice(c *gc.C) {
	defer s.setup(c, 2).Finish()

	// A WaitGroup for this test to synchronize when the
	// machine notifications are sent.  The 2nd group must
	// be after machine 0 gets Life() == Alive.
	var group sync.WaitGroup
	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachinesWaitGroup([][]string{{"0", "1"}, {"0"}}, &group),
		s.expectFacadeMachineTag(0),
		s.expectFacadeMachineTag(1),
		s.notifyMachineAppLXDProfile(0, 1),
		s.notifyMachineAppLXDProfile(1, 1),
		s.expectAliveAndSetModificationStatusIdle(1),
		s.expectMachineCharmProfilingInfo(0, 2),
		s.expectMachineCharmProfilingInfo(1, 2),
		s.expectLXDProfileNamesTrue,
		s.expectLXDProfileNamesTrue,
		s.expectMachineAliveStatusIdleMachineDead(0, &group),
	)
	s.cleanKill(c, w)
}

func (s *workerEnvironSuite) TestNoChangeFoundOne(c *gc.C) {
	defer s.setup(c, 1).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{{"0"}}),
		s.expectFacadeMachineTag(0),
		s.notifyMachineAppLXDProfile(0, 1),
		s.expectCharmProfilingInfoSimpleNoChange(0),
	)
	s.cleanKill(c, w)
}

func (s *workerEnvironSuite) TestNoMachineFound(c *gc.C) {
	defer s.setup(c, 1).Finish()

	w, err := s.workerErrorForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{{"0"}}),
		s.expectFacadeReturnsNoMachine,
	)

	// Since we don't use cleanKill() nor errorKill()
	// here, but do waitDone() before checking errors.
	s.waitDone(c)

	// This test had intermittent failures, one of the
	// two following would occur.  The 2nd is what we're
	// looking for.  Please improve this test if you're
	// able.
	if err != nil {
		c.Assert(err, gc.ErrorMatches, "catacomb .* is dying")
	} else {
		err = workertest.CheckKill(c, w)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

func (s *workerEnvironSuite) TestCharmProfilingInfoNotProvisioned(c *gc.C) {
	defer s.setup(c, 1).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{{"0"}}),
		s.expectFacadeMachineTag(0),
		s.notifyMachineAppLXDProfile(0, 1),
		s.expectCharmProfileInfoNotProvisioned(0),
	)

	err := s.errorKill(c, w)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
}

func (s *workerSuite) setup(c *gc.C, machineCount int) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.facade = mocks.NewMockInstanceMutaterAPI(ctrl)
	s.broker = mocks.NewMockLXDProfiler(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.machinesWorker = workermocks.NewMockWorker(ctrl)

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
func (s *workerSuite) workerForScenario(c *gc.C, behaviours ...func()) worker.Worker {
	config := instancemutater.Config{
		Facade:                 s.facade,
		Logger:                 s.logger,
		Broker:                 s.broker,
		AgentConfig:            s.agentConfig,
		Tag:                    s.machineTag,
		GetRequiredLXDProfiles: s.getRequiredLXDProfiles,
	}

	for _, b := range behaviours {
		b()
	}

	w, err := s.newWorkerFunc(config)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

// workerErrorForScenario creates worker config based on the suite's mocks.
// Any supplied behaviour functions are executed, then a new worker is
// started and returned with any error in creation.
func (s *workerSuite) workerErrorForScenario(c *gc.C, behaviours ...func()) (worker.Worker, error) {
	config := instancemutater.Config{
		Facade:      s.facade,
		Logger:      s.logger,
		Broker:      s.broker,
		AgentConfig: s.agentConfig,
		Tag:         s.machineTag,
	}

	for _, b := range behaviours {
		b()
	}

	return s.newWorkerFunc(config)
}

func (s *workerSuite) expectFacadeMachineTag(machine int) func() {
	return func() {
		tag := names.NewMachineTag(strconv.Itoa(machine))
		s.facade.EXPECT().Machine(tag).Return(s.machine[machine], nil).AnyTimes()
		s.machine[machine].EXPECT().Tag().Return(tag).AnyTimes()
	}
}

func (s *workerSuite) expectFacadeReturnsNoMachine() {
	do := s.workGroupAddGetDoneFunc()
	s.facade.EXPECT().Machine(s.machineTag).Return(nil, errors.NewNotFound(nil, "machine")).Do(do)
}

func (s *workerSuite) expectCharmProfilingInfoSimpleNoChange(machine int) func() {
	return func() {
		do := s.workGroupAddGetDoneFunc()
		s.machine[machine].EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, nil).Do(do)
	}
}

func (s *workerSuite) workGroupAddGetDoneFunc() func(_ ...interface{}) {
	s.doneWG.Add(1)
	return func(_ ...interface{}) { s.doneWG.Done() }
}

func (s *workerSuite) expectLXDProfileNamesTrue() {
	s.broker.EXPECT().LXDProfileNames("juju-23423-0").Return([]string{"default", "juju-testing", "juju-testing-one-2"}, nil)
}

func (s *workerSuite) expectMachineCharmProfilingInfo(machine, rev int) func() {
	return s.expectCharmProfilingInfo(s.machine[machine], rev)
}

func (s *workerSuite) expectCharmProfilingInfo(mock *mocks.MockMutaterMachine, rev int) func() {
	return func() {
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
}

func (s *workerSuite) expectCharmProfileInfoNotProvisioned(machine int) func() {
	return func() {
		do := s.workGroupAddGetDoneFunc()
		s.machine[machine].EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, errors.NotProvisionedf("machine 0")).Do(do)
	}
}

func (s *workerSuite) expectAliveAndSetModificationStatusIdle(machine int) func() {
	return func() {
		mExp := s.machine[machine].EXPECT()
		mExp.Refresh().Return(nil)
		mExp.Life().Return(params.Alive)
		mExp.SetModificationStatus(status.Idle, "", nil).Return(nil)
	}
}

func (s *workerSuite) expectMachineAliveStatusIdleMachineDead(machine int, group *sync.WaitGroup) func() {
	return func() {
		mExp := s.machine[machine].EXPECT()

		group.Add(1)
		notificationSync := func(_ ...interface{}) { group.Done() }

		mExp.Refresh().Return(nil).Times(2)
		o1 := mExp.Life().Return(params.Alive).Do(notificationSync)

		mExp.SetModificationStatus(status.Idle, "", nil).Return(nil)

		do := s.workGroupAddGetDoneFunc()
		s.machine[0].EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil)
		s.machine[1].EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil).Do(do)

		s.doneWG.Add(1)
		mExp.Life().Return(params.Dead).After(o1).Do(do)
	}
}

func (s *workerSuite) expectModificationStatusApplied(machine int) func() {
	return func() {
		do := s.workGroupAddGetDoneFunc()
		s.machine[machine].EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil).Do(do)
	}
}

func (s *workerSuite) expectAssignLXDProfiles() {
	profiles := []string{"default", "juju-testing", "juju-testing-one-3"}
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerSuite) expectSetCharmProfiles(machine int) func() {
	return func() {
		s.machine[machine].EXPECT().SetCharmProfiles([]string{"default", "juju-testing", "juju-testing-one-3"})
	}
}

// notifyMachines returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notifyMachines(values [][]string) func() {
	ch := make(chan []string)

	return func() {
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
}

func (s *workerSuite) notifyMachinesWaitGroup(values [][]string, group *sync.WaitGroup) func() {
	ch := make(chan []string)

	return func() {
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
}

// notifyAppLXDProfile returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notifyMachineAppLXDProfile(machine, times int) func() {
	return s.notifyAppLXDProfile(s.machine[machine], machine, times)
}

func (s *workerContainerSuite) notifyContainerAppLXDProfile(times int) func() {
	return s.notifyAppLXDProfile(s.container, 0, times)
}

func (s *workerSuite) notifyAppLXDProfile(mock *mocks.MockMutaterMachine, which, times int) func() {
	ch := make(chan struct{})

	return func() {
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

		mock.EXPECT().WatchApplicationLXDProfiles().Return(
			&fakeNotifyWatcher{
				Worker: w,
				ch:     ch,
			}, nil)
	}
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
		ch <- struct{}{}
	}()

	select {
	case <-ch:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
}

type loggerSuite struct {
	testing.IsolationSuite

	logger *mocks.MockLogger
}

var _ = gc.Suite(&loggerSuite{})

// ignoreLogging turns the suite's mock Logger into a sink, with no validation.
// Logs are still emitted via the test Logger.
func (s *loggerSuite) ignoreLogging(c *gc.C) func() {
	warnIt := func(message string, args ...interface{}) { logIt(c, loggo.WARNING, message, args) }
	debugIt := func(message string, args ...interface{}) { logIt(c, loggo.DEBUG, message, args) }
	errorIt := func(message string, args ...interface{}) { logIt(c, loggo.ERROR, message, args) }
	traceIt := func(message string, args ...interface{}) { logIt(c, loggo.TRACE, message, args) }

	return func() {
		e := s.logger.EXPECT()
		e.Warningf(gomock.Any(), gomock.Any()).AnyTimes().Do(warnIt)
		e.Debugf(gomock.Any(), gomock.Any()).AnyTimes().Do(debugIt)
		e.Errorf(gomock.Any(), gomock.Any()).AnyTimes().Do(errorIt)
		e.Tracef(gomock.Any(), gomock.Any()).AnyTimes().Do(traceIt)
	}
}

func logIt(c *gc.C, level loggo.Level, message string, args interface{}) {
	var nArgs []interface{}
	var ok bool
	if nArgs, ok = args.([]interface{}); ok {
		nArgs = append([]interface{}{level}, nArgs...)
	} else {
		nArgs = append([]interface{}{level}, args)
	}

	c.Logf("%s "+message, nArgs...)
}

type workerContainerSuite struct {
	workerSuite

	containerTag names.Tag
	container    *mocks.MockMutaterMachine
}

var _ = gc.Suite(&workerContainerSuite{})

func (s *workerContainerSuite) SetUpTest(c *gc.C) {
	s.workerSuite.SetUpTest(c)

	s.containerTag = names.NewMachineTag("0/lxd/0")
	s.newWorkerFunc = instancemutater.NewContainerWorker
	s.getRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default"}
	}
}

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole instance mutator scenario, from start
// below to compose a test of the whole instance mutator scenario, from start
// to finish for a ContainerWorker.
func (s *workerContainerSuite) TestFullWorkflow(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyContainers(0, [][]string{{"0/lxd/0"}}),
		s.expectFacadeMachineTag(0),
		s.expectFacadeContainerTag,
		s.notifyContainerAppLXDProfile(1),
		s.expectContainerTag,
		s.expectContainerCharmProfilingInfo(3),
		s.expectLXDProfileNamesTrue,
		s.expectContainerSetCharmProfiles,
		s.expectAssignLXDProfiles,
		s.expectContainerAliveAndSetModificationStatusIdle,
		s.expectContainerModificationStatusApplied,
	)
	s.cleanKill(c, w)
}

func (s *workerContainerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.workerSuite.setup(c, 1)
	s.container = mocks.NewMockMutaterMachine(ctrl)
	return ctrl
}

func (s *workerContainerSuite) expectFacadeContainerTag() {
	s.facade.EXPECT().Machine(s.containerTag).Return(s.container, nil).AnyTimes()
	s.container.EXPECT().Tag().Return(s.containerTag).AnyTimes()
}

func (s *workerContainerSuite) expectContainerTag() {
	s.container.EXPECT().Tag().Return(s.containerTag).AnyTimes()
}

func (s *workerContainerSuite) expectContainerCharmProfilingInfo(rev int) func() {
	return s.expectCharmProfilingInfo(s.container, rev)
}

func (s *workerContainerSuite) expectContainerAliveAndSetModificationStatusIdle() {
	cExp := s.container.EXPECT()
	cExp.Refresh().Return(nil)
	cExp.Life().Return(params.Alive)
	cExp.SetModificationStatus(status.Idle, gomock.Any(), gomock.Any()).Return(nil)
}

func (s *workerContainerSuite) expectContainerModificationStatusApplied() {
	do := s.workGroupAddGetDoneFunc()
	s.container.EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil).Do(do)
}

func (s *workerContainerSuite) expectAssignLXDProfiles() {
	profiles := []string{"default", "juju-testing-one-3"}
	s.broker.EXPECT().AssignLXDProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerContainerSuite) expectContainerSetCharmProfiles() {
	s.container.EXPECT().SetCharmProfiles([]string{"default", "juju-testing-one-3"})
}

// notifyContainers returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerContainerSuite) notifyContainers(machine int, values [][]string) func() {
	ch := make(chan []string)

	return func() {
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
