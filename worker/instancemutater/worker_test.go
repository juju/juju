// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
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
			description: "Test no logger",
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
			err: "nil Environ not valid",
		},
		{
			description: "Test no agent",
			config: instancemutater.Config{
				Logger:  mocks.NewMockLogger(ctrl),
				Facade:  mocks.NewMockInstanceMutaterAPI(ctrl),
				Environ: mocks.NewMockEnviron(ctrl),
			},
			err: "nil AgentConfig not valid",
		},
		{
			description: "Test no tag",
			config: instancemutater.Config{
				Logger:      mocks.NewMockLogger(ctrl),
				Facade:      mocks.NewMockInstanceMutaterAPI(ctrl),
				Environ:     mocks.NewMockEnviron(ctrl),
				AgentConfig: mocks.NewMockConfig(ctrl),
			},
			err: "nil Tag not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *workerConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.Config{
		Facade:      mocks.NewMockInstanceMutaterAPI(ctrl),
		Logger:      mocks.NewMockLogger(ctrl),
		Environ:     mocks.NewMockEnviron(ctrl),
		AgentConfig: mocks.NewMockConfig(ctrl),
		Tag:         names.MachineTag{},
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}

type workerSuite struct {
	testing.IsolationSuite

	logger              *mocks.MockLogger
	facade              *mocks.MockInstanceMutaterAPI
	environ             environShim
	agentConfig         *mocks.MockConfig
	machine             *mocks.MockMutaterMachine
	machineTag          names.Tag
	machinesWorker      *workermocks.MockWorker
	appLXDProfileWorker *workermocks.MockWorker

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.done = make(chan struct{})
	s.machineTag = names.NewMachineTag("0")
}

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole instance mutator scenario, from start
// to finish.
func (s *workerSuite) TestFullWorkflow(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{
			{"0"},
		}, s.noopDone),
		s.expectFacadeMachineTag,
		s.notifyAppLXDProfile(1, s.closeDone),
		s.expectMachineTag,
		s.expectCharmProfilingInfo(3),
		s.expectLXDProfileNames,
		s.expectSetCharmProfiles,
		s.expectAssignProfiles,
		s.expectModificationStatusIdle,
	)
	s.cleanKill(c, w)
}

func (s *workerSuite) TestVerifyCurrentProfilesTrue(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{
			{"0"},
		}, s.noopDone),
		s.expectFacadeMachineTag,
		s.notifyAppLXDProfile(1, s.closeDone),
		s.expectMachineTag,
		s.expectCharmProfilingInfo(2),
		s.expectLXDProfileNames,
		s.expectModificationStatusIdle,
	)
	s.cleanKill(c, w)
}

func (s *workerSuite) TestNoChangeFoundOne(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{
			{"0"},
		}, s.noopDone),
		s.expectFacadeMachineTag,
		s.notifyAppLXDProfile(1, s.closeDone),
		s.expectMachineTag,
		s.expectCharmProfilingInfoSimpleNoChange,
	)
	s.cleanKill(c, w)
}

func (s *workerSuite) TestNoMachineFound(c *gc.C) {
	defer s.setup(c).Finish()

	w, err := s.workerErrorForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{
			{"0"},
		}, s.closeDone),
		s.expectFacadeReturnsNoMachine,
	)

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

func (s *workerSuite) TestCharmProfilingInfoNotProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

	w := s.workerForScenario(c,
		s.ignoreLogging(c),
		s.notifyMachines([][]string{
			{"0"},
		}, s.noopDone),
		s.expectFacadeMachineTag,
		s.notifyAppLXDProfile(1, s.closeDone),
		s.expectMachineTag,
		s.expectCharmProfileInfoNotProvisioned,
	)

	err := s.errorKill(c, w)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
}

func (s *workerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	s.facade = mocks.NewMockInstanceMutaterAPI(ctrl)
	s.environ = environShim{
		MockEnviron:     mocks.NewMockEnviron(ctrl),
		MockLXDProfiler: mocks.NewMockLXDProfiler(ctrl),
	}
	s.agentConfig = mocks.NewMockConfig(ctrl)
	s.machine = mocks.NewMockMutaterMachine(ctrl)
	s.machinesWorker = workermocks.NewMockWorker(ctrl)
	s.appLXDProfileWorker = workermocks.NewMockWorker(ctrl)

	return ctrl
}

func (s *workerSuite) noopDone() {
	// do nothing with the done channel
}

func (s *workerSuite) closeDone() {
	close(s.done)
}

// workerForScenario creates worker config based on the suite's mocks.
// Any supplied behaviour functions are executed, then a new worker
// is started successfully and returned.
func (s *workerSuite) workerForScenario(c *gc.C, behaviours ...func()) worker.Worker {
	config := instancemutater.Config{
		Facade:      s.facade,
		Logger:      s.logger,
		Environ:     s.environ,
		AgentConfig: s.agentConfig,
		Tag:         s.machineTag,
	}

	for _, b := range behaviours {
		b()
	}

	w, err := instancemutater.NewWorker(config)
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
		Environ:     s.environ,
		AgentConfig: s.agentConfig,
		Tag:         s.machineTag,
	}

	for _, b := range behaviours {
		b()
	}

	return instancemutater.NewWorker(config)
}

func (s *workerSuite) expectFacadeMachineTag() {
	s.facade.EXPECT().Machine(s.machineTag).Return(s.machine, nil)
	s.machine.EXPECT().Tag().Return(s.machineTag)
}

func (s *workerSuite) expectFacadeReturnsNoMachine() {
	s.facade.EXPECT().Machine(s.machineTag).Return(nil, errors.NewNotFound(nil, "machine"))
}

func (s *workerSuite) expectMachineTag() {
	s.machine.EXPECT().Tag().Return(s.machineTag).AnyTimes()
}

func (s *workerSuite) expectCharmProfilingInfoSimpleNoChange() {
	s.machine.EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, nil)
}

func (s *workerSuite) expectLXDProfileNames() {
	s.environ.MockLXDProfiler.EXPECT().LXDProfileNames("juju-23423-0").Return([]string{"default", "juju-testing", "juju-testing-one-2"}, nil)
}

func (s *workerSuite) expectCharmProfilingInfo(rev int) func() {
	return func() {
		s.machine.EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{
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

func (s *workerSuite) expectCharmProfileInfoNotProvisioned() {
	s.machine.EXPECT().CharmProfilingInfo().Return(&apiinstancemutater.UnitProfileInfo{}, errors.NotProvisionedf("machine 0"))
}

func (s *workerSuite) expectModificationStatusIdle() {
	s.machine.EXPECT().SetModificationStatus(status.Idle, "", nil).Return(nil)
}

func (s *workerSuite) expectAssignProfiles() {
	profiles := []string{"default", "juju-testing", "juju-testing-one-3"}
	s.environ.MockLXDProfiler.EXPECT().AssignProfiles("juju-23423-0", profiles, gomock.Any()).Return(profiles, nil)
}

func (s *workerSuite) expectSetCharmProfiles() {
	s.machine.EXPECT().SetCharmProfiles([]string{"default", "juju-testing", "juju-testing-one-3"})
}

// notifyMachines returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notifyMachines(values [][]string, doneFn func()) func() {
	ch := make(chan []string)

	return func() {
		go func() {
			for _, v := range values {
				ch <- v
			}
			doneFn()
		}()

		s.machinesWorker.EXPECT().Kill().AnyTimes()
		s.machinesWorker.EXPECT().Wait().Return(nil).AnyTimes()

		s.facade.EXPECT().WatchModelMachines().Return(
			&fakeStringsWatcher{
				Worker: s.machinesWorker,
				ch:     ch,
			}, nil)
	}
}

// notifyAppLXDProfile returns a suite behaviour that will cause the instance mutator
// watcher to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notifyAppLXDProfile(times int, doneFn func()) func() {
	ch := make(chan struct{})

	return func() {
		go func() {
			for i := 0; i < times; i += 1 {
				ch <- struct{}{}
			}
			doneFn()
		}()

		s.appLXDProfileWorker.EXPECT().Kill().AnyTimes()
		s.appLXDProfileWorker.EXPECT().Wait().Return(nil).AnyTimes()

		s.machine.EXPECT().WatchApplicationLXDProfiles().Return(
			&fakeNotifyWatcher{
				Worker: s.appLXDProfileWorker,
				ch:     ch,
			}, nil)
	}
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *workerSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}

// errorKill waits for notifications to be processed, then waits for the input
// worker to be killed.  Any error is returned to the caller. If either ops
// time out, the test fails.
func (s *workerSuite) errorKill(c *gc.C, w worker.Worker) error {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	return workertest.CheckKill(c, w)
}

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
// Logs are still emitted via the test logger.
func (s *workerSuite) ignoreLogging(c *gc.C) func() {
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

type environShim struct {
	*mocks.MockEnviron
	*mocks.MockLXDProfiler
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
