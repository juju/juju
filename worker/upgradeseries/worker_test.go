// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/testing"
	workermocks "github.com/juju/juju/worker/mocks"
	"github.com/juju/juju/worker/upgradeseries"
	. "github.com/juju/juju/worker/upgradeseries/mocks"
)

type fakeWatcher struct {
	worker.Worker
	ch <-chan struct{}
}

func (w *fakeWatcher) Changes() watcher.NotifyChannel {
	return w.ch
}

type workerSuite struct {
	testing.BaseSuite

	logger       *MockLogger
	facade       *MockFacade
	service      *MockServiceAccess
	upgrader     *MockUpgrader
	notifyWorker *workermocks.MockWorker

	wordPressAgent *MockAgentService
	mySQLAgent     *MockAgentService

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.done = make(chan struct{})
}

// TestFullWorkflow uses the the expectation scenarios from each of the tests
// below to compose a test of the whole upgrade-series scenario, from start
// to finish.
func (s *workerSuite) TestFullWorkflow(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// TODO (manadart 2018-09-05): The idea of passing behaviours into a
	// scenario (as below) evolved so as to make itself redundant.
	// All of the anonymous funcs passed could be called directly on the suite
	// here, with the same effect and greater clarity.

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(6),
		s.expectMachinePrepareStartedUnitsNotPrepareCompleteNoAction,
		s.expectMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete,
		s.expectMachineCompleteStartedUnitsPrepareCompleteUnitsStarted,
		s.expectMachineCompleteStartedUnitsCompleteProgressComplete,
		s.expectMachineCompletedFinishUpgradeSeries,
		s.expectLockNotFoundNoAction)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesNotStarted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) TestLockNotFoundNoAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1),
		s.expectLockNotFoundNoAction)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesNotStarted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectLockNotFoundNoAction() {
	// If the lock is not found, no further processing occurs.
	// This is the only call we expect to see.
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesStatus(""), errors.NewNotFound(nil, "nope"))
}

func (s *workerSuite) TestCompleteNoAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the workflow is completed, no further processing occurs.
	// This is the only call we expect to see.
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesPrepareCompleted, nil)

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1))

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesPrepareCompleted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) TestMachinePrepareStartedUnitsNotPrepareCompleteNoAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1), s.expectMachinePrepareStartedUnitsNotPrepareCompleteNoAction)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesPrepareStarted,
		"prepared units": []string{"wordpress/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachinePrepareStartedUnitsNotPrepareCompleteNoAction() {
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesPrepareStarted, nil)
	// Only one of the two units has completed preparation.
	s.expectUnitsPrepared("wordpress/0")

	// After comparing the prepare-complete units with the services,
	// no further action is taken.
	s.expectServiceDiscovery(false)
}

func (s *workerSuite) TestMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1),
		s.expectMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesPrepareStarted,
		"prepared units": []string{"wordpress/0", "mysql/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachinePrepareStartedUnitFilesWrittenProgressPrepareComplete() {
	exp := s.facade.EXPECT()

	exp.MachineStatus().Return(model.UpgradeSeriesPrepareStarted, nil)
	s.expectUnitsPrepared("wordpress/0", "mysql/0")
	exp.TargetSeries().Return("xenial", nil)

	s.upgrader.EXPECT().PerformUpgrade().Return(nil)

	exp.SetMachineStatus(model.UpgradeSeriesPrepareCompleted, gomock.Any()).Return(nil)

	s.expectServiceDiscovery(false)
}

func (s *workerSuite) TestMachineCompleteStartedUnitsPrepareCompleteUnitsStarted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1),
		s.expectMachineCompleteStartedUnitsPrepareCompleteUnitsStarted)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesCompleteStarted,
		"prepared units": []string{"wordpress/0", "mysql/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachineCompleteStartedUnitsPrepareCompleteUnitsStarted() {
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesCompleteStarted, nil)
	s.expectUnitsPrepared("wordpress/0", "mysql/0")
	s.facade.EXPECT().StartUnitCompletion(gomock.Any()).Return(nil)

	s.expectServiceDiscovery(true)

	s.wordPressAgent.EXPECT().Running().Return(false, nil)
	s.wordPressAgent.EXPECT().Start().Return(nil)

	s.mySQLAgent.EXPECT().Running().Return(true, nil)
}

func (s *workerSuite) TestMachineCompleteStartedNoUnitsProgressComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.facade.EXPECT()
	exp.MachineStatus().Return(model.UpgradeSeriesCompleteStarted, nil)

	// Machine with no units - API calls return none, no services discovered.
	exp.UnitsPrepared().Return(nil, nil)
	exp.UnitsCompleted().Return(nil, nil)
	s.service.EXPECT().ListServices().Return(nil, nil).Times(2)

	// Progress directly to completed.
	exp.SetMachineStatus(model.UpgradeSeriesCompleted, gomock.Any()).Return(nil)

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1))

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesCompleteStarted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) TestMachineCompleteStartedUnitsCompleteProgressComplete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1),
		s.expectMachineCompleteStartedUnitsCompleteProgressComplete)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status":  model.UpgradeSeriesCompleteStarted,
		"completed units": []string{"wordpress/0", "mysql/0"},
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachineCompleteStartedUnitsCompleteProgressComplete() {
	exp := s.facade.EXPECT()

	exp.MachineStatus().Return(model.UpgradeSeriesCompleteStarted, nil)
	// No units are in the prepare-complete state.
	// They have completed their workflow.
	s.expectUnitsPrepared()
	s.facade.EXPECT().UnitsCompleted().Return([]names.UnitTag{
		names.NewUnitTag("wordpress/0"),
		names.NewUnitTag("mysql/0"),
	}, nil)
	exp.SetMachineStatus(model.UpgradeSeriesCompleted, gomock.Any()).Return(nil)

	// TODO (manadart 2018-08-22): Modify the tested code so that the services
	// are detected just the one time.
	s.expectServiceDiscovery(false)
	s.expectServiceDiscovery(false)
}

func (s *workerSuite) TestMachineCompletedFinishUpgradeSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.workerForScenario(c, s.ignoreLogging(c), s.notify(1),
		s.expectMachineCompletedFinishUpgradeSeries)

	s.cleanKill(c, w)
	expected := map[string]interface{}{
		"machine status": model.UpgradeSeriesCompleted,
	}
	c.Check(w.(worker.Reporter).Report(), gc.DeepEquals, expected)
}

func (s *workerSuite) expectMachineCompletedFinishUpgradeSeries() {
	s.patchHost("xenial")

	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesCompleted, nil)
	s.facade.EXPECT().FinishUpgradeSeries("xenial").Return(nil)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.facade = NewMockFacade(ctrl)
	s.service = NewMockServiceAccess(ctrl)
	s.upgrader = NewMockUpgrader(ctrl)
	s.notifyWorker = workermocks.NewMockWorker(ctrl)
	s.wordPressAgent = NewMockAgentService(ctrl)
	s.mySQLAgent = NewMockAgentService(ctrl)

	return ctrl
}

// workerForScenario creates worker config based on the suite's mocks.
// Any supplied behaviour functions are executed,
// then a new worker is started and returned.
func (s *workerSuite) workerForScenario(c *gc.C, behaviours ...func()) worker.Worker {
	cfg := upgradeseries.Config{
		Logger:          s.logger,
		FacadeFactory:   func(_ names.Tag) upgradeseries.Facade { return s.facade },
		Tag:             names.NewMachineTag("0"),
		Service:         s.service,
		UpgraderFactory: func(_ string) (upgradeseries.Upgrader, error) { return s.upgrader, nil },
	}

	for _, b := range behaviours {
		b()
	}

	w, err := upgradeseries.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

// expectUnitsCompleted represents the scenario where the input unit names
// have completed their upgrade-series preparation.
func (s *workerSuite) expectUnitsPrepared(units ...string) {
	tags := make([]names.UnitTag, len(units))
	for i, u := range units {
		tags[i] = names.NewUnitTag(u)
	}
	s.facade.EXPECT().UnitsPrepared().Return(tags, nil)
}

// expectServiceDiscovery is a convenience method for expectations that mimic
// detection of unit agent services on the local machine.
func (s *workerSuite) expectServiceDiscovery(discover bool) {
	sExp := s.service.EXPECT()

	sExp.ListServices().Return([]string{
		"jujud-unit-wordpress-0",
		"jujud-unit-mysql-0",
		"jujud-machine-0",
	}, nil)

	// If discover is false, we are mocking a scenario where the worker does
	// not interact with the services.
	if !discover {
		return
	}

	// Note that the machine agent service listed above is ignored as non-unit.
	sExp.DiscoverService("jujud-unit-wordpress-0").Return(s.wordPressAgent, nil)
	sExp.DiscoverService("jujud-unit-mysql-0").Return(s.mySQLAgent, nil)
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

func (s *workerSuite) patchHost(series string) {
	upgradeseries.PatchHostSeries(s, series)
}

// notify returns a suite behaviour that will cause the upgrade-series watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *workerSuite) notify(times int) func() {
	ch := make(chan struct{})

	return func() {
		go func() {
			for i := 0; i < times; i++ {
				ch <- struct{}{}
			}
			close(s.done)
		}()

		s.notifyWorker.EXPECT().Kill().AnyTimes()
		s.notifyWorker.EXPECT().Wait().Return(nil).AnyTimes()

		s.facade.EXPECT().WatchUpgradeSeriesNotifications().Return(
			&fakeWatcher{
				Worker: s.notifyWorker,
				ch:     ch,
			}, nil)
	}
}

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
// Logs are still emitted via the test logger.
func (s *workerSuite) ignoreLogging(c *gc.C) func() {
	debugIt := func(message string, args ...interface{}) { logIt(c, loggo.DEBUG, message, args) }
	infoIt := func(message string, args ...interface{}) { logIt(c, loggo.INFO, message, args) }
	warnIt := func(message string, args ...interface{}) { logIt(c, loggo.WARNING, message, args) }
	errorIt := func(message string, args ...interface{}) { logIt(c, loggo.ERROR, message, args) }

	return func() {
		e := s.logger.EXPECT()
		e.Debugf(gomock.Any(), gomock.Any()).AnyTimes().Do(debugIt)
		e.Infof(gomock.Any(), gomock.Any()).AnyTimes().Do(infoIt)
		e.Warningf(gomock.Any(), gomock.Any()).AnyTimes().Do(errorIt)
		e.Errorf(gomock.Any(), gomock.Any()).AnyTimes().Do(warnIt)
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
