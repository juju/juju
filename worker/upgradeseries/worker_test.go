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
	notifyWorker *MockWorker
	service      *MockServiceAccess

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
}

var _ = gc.Suite(&workerSuite{})

type suiteBehaviour func(*workerSuite)

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.done = make(chan struct{})
}

func (s *workerSuite) TestLockNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	// If the lock is not found. no further processing occurs.
	// This is the only call we expect to see.
	s.facade.EXPECT().MachineStatus().Return(model.UpgradeSeriesStatus(""), errors.NewNotFound(nil, "nope"))

	w := s.newWorker(c, ctrl, ignoreLogging(c), notify(1))
	s.cleanKill(c, w)
}

func (s *workerSuite) TestInconsistentStateNoChange(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	exp := s.facade.EXPECT()
	exp.MachineStatus().Return(model.PrepareStarted, nil)
	exp.UpgradeSeriesStatus().Return([]string{"nope"}, nil)

	w := s.newWorker(c, ctrl, ignoreLogging(c), notify(1))
	s.cleanKill(c, w)
}

func (s *workerSuite) TestPrepareStatusCompleteUnitsStoppedPrepareMachine(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	agentWordpress := NewMockAgentService(ctrl)
	agentMySQL := NewMockAgentService(ctrl)

	fExp := s.facade.EXPECT()
	fExp.MachineStatus().Return(model.PrepareStarted, nil)
	fExp.UpgradeSeriesStatus().Return([]string{string(model.PrepareCompleted), string(model.PrepareCompleted)}, nil)
	fExp.SetMachineStatus(model.PrepareMachine).Return(nil)

	sExp := s.service.EXPECT()
	sExp.ListServices().Return([]string{
		"jujud-unit-wordpress-0",
		"jujud-unit-mysql-0",
		"jujud-machine-0",
	}, nil)
	sExp.DiscoverService("jujud-unit-wordpress-0").Return(agentWordpress, nil)
	sExp.DiscoverService("jujud-unit-mysql-0").Return(agentMySQL, nil)

	agentWordpress.EXPECT().Running().Return(true, nil)
	agentWordpress.EXPECT().Stop().Return(nil)

	agentMySQL.EXPECT().Running().Return(false, nil)

	w := s.newWorker(c, ctrl, ignoreLogging(c), notify(1))
	s.cleanKill(c, w)
}

func (s *workerSuite) TestPrepareMachineUnitFilesWrittenPrepareComplete(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	fExp := s.facade.EXPECT()
	fExp.MachineStatus().Return(model.PrepareMachine, nil)
	fExp.UpgradeSeriesStatus().Return([]string{string(model.PrepareCompleted), string(model.PrepareCompleted)}, nil)

	// TODO (manadart 2018-08-09): Assertions for service unit manipulation.

	fExp.SetMachineStatus(model.PrepareCompleted).Return(nil)

	sExp := s.service.EXPECT()
	sExp.ListServices().Return([]string{
		"jujud-unit-wordpress-0",
		"jujud-unit-mysql-0",
		"jujud-machine-0",
	}, nil)

	w := s.newWorker(c, ctrl, ignoreLogging(c), notify(1))
	s.cleanKill(c, w)
}

func (s *workerSuite) TestCompleteStartedUnitsNotStartedUnitsStarted(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	agentWordpress := NewMockAgentService(ctrl)
	agentMySQL := NewMockAgentService(ctrl)

	fExp := s.facade.EXPECT()
	fExp.MachineStatus().Return(model.CompleteStarted, nil)
	fExp.UpgradeSeriesStatus().Return([]string{string(model.PrepareCompleted), string(model.PrepareCompleted)}, nil)
	fExp.StartUnitCompletion().Return(nil)

	sExp := s.service.EXPECT()
	sExp.ListServices().Return([]string{
		"jujud-unit-wordpress-0",
		"jujud-unit-mysql-0",
		"jujud-machine-0",
	}, nil)
	sExp.DiscoverService("jujud-unit-wordpress-0").Return(agentWordpress, nil)
	sExp.DiscoverService("jujud-unit-mysql-0").Return(agentMySQL, nil)

	agentWordpress.EXPECT().Running().Return(false, nil)
	agentWordpress.EXPECT().Start().Return(nil)

	agentMySQL.EXPECT().Running().Return(true, nil)

	w := s.newWorker(c, ctrl, ignoreLogging(c), notify(1))
	s.cleanKill(c, w)
}

func (s *workerSuite) TestCompleteStartedUnitsCompleteComplete(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	fExp := s.facade.EXPECT()
	fExp.MachineStatus().Return(model.CompleteStarted, nil)
	fExp.UpgradeSeriesStatus().Return([]string{string(model.Completed), string(model.Completed)}, nil)
	fExp.SetMachineStatus(model.Completed).Return(nil)

	w := s.newWorker(c, ctrl, ignoreLogging(c), notify(1))
	s.cleanKill(c, w)
}

func (s *workerSuite) setupMocks(ctrl *gomock.Controller) {
	s.logger = NewMockLogger(ctrl)
	s.facade = NewMockFacade(ctrl)
	s.notifyWorker = NewMockWorker(ctrl)
	s.service = NewMockServiceAccess(ctrl)
}

// newWorker creates worker dependency mocks using the input controller.
// Any supplied behaviour functions are applied to the suite, then a new worker
// is started and returned.
func (s *workerSuite) newWorker(c *gc.C, ctrl *gomock.Controller, behaviours ...suiteBehaviour) worker.Worker {
	cfg := upgradeseries.Config{
		Logger:        s.logger,
		FacadeFactory: func(_ names.Tag) upgradeseries.Facade { return s.facade },
		Tag:           names.NewMachineTag("0"),
		Service:       s.service,
	}

	for _, b := range behaviours {
		b(s)
	}

	w, err := upgradeseries.NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
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

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
// Logs are still emitted via the test logger.
func ignoreLogging(c *gc.C) suiteBehaviour {
	logIt := func(level loggo.Level, message string, args ...interface{}) {
		nArgs := append([]interface{}{level}, args)
		c.Logf("%s "+message, nArgs...)
	}
	warnIt := func(message string, args ...interface{}) {
		logIt(loggo.WARNING, message, args...)
	}
	errorIt := func(message string, args ...interface{}) {
		logIt(loggo.ERROR, message, args...)
	}

	return func(s *workerSuite) {
		e := s.logger.EXPECT()
		e.Logf(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Do(logIt)
		e.Errorf(gomock.Any(), gomock.Any()).AnyTimes().Do(warnIt)
		e.Warningf(gomock.Any(), gomock.Any()).AnyTimes().Do(errorIt)
	}
}

// notify returns a suite behaviour that will cause the upgrade-series watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func notify(times int) suiteBehaviour {
	ch := make(chan struct{})

	return func(s *workerSuite) {
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
