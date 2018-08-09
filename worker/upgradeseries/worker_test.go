// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/upgradeseries"
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

func (s *workerSuite) TestInconsistentStateNoChange(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.facade.EXPECT().UpgradeSeriesStatus(model.PrepareStatus).Return([]string{"nope"}, nil)
	s.facade.EXPECT().UpgradeSeriesStatus(model.CompleteStatus).Return([]string{"nope"}, nil)

	w := s.newWorker(c, ctrl, ignoreLogging, notify(1))
	<-s.done
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestPrepareStatusCompleteUnitsStopped(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	agentWordpress := NewMockAgentService(ctrl)
	agentMySQL := NewMockAgentService(ctrl)

	s.facade.EXPECT().UpgradeSeriesStatus(model.PrepareStatus).Return([]string{"Completed", "Completed"}, nil)
	s.facade.EXPECT().UpgradeSeriesStatus(model.CompleteStatus).Return([]string{"NotStarted", "NotStarted"}, nil)
	// Expect an upgrade to machine status.

	exp := s.service.EXPECT()
	exp.ListServices().Return([]string{
		"jujud-unit-wordpress-0",
		"jujud-unit-mysql-0",
		"jujud-machine-0",
	}, nil)
	exp.DiscoverService("jujud-unit-wordpress-0").Return(agentWordpress, nil)
	exp.DiscoverService("jujud-unit-mysql-0").Return(agentMySQL, nil)

	agentWordpress.EXPECT().Running().Return(true, nil)
	agentWordpress.EXPECT().Stop().Return(nil)

	agentMySQL.EXPECT().Running().Return(true, nil)
	agentMySQL.EXPECT().Stop().Return(nil)

	w := s.newWorker(c, ctrl, ignoreLogging, notify(1))
	<-s.done
	workertest.CleanKill(c, w)
}

// ignoreLogging turns the suite's mock logger into a sink, with no validation.
func ignoreLogging(s *workerSuite) {
	e := s.logger.EXPECT()
	e.Logf(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	e.Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	e.Warningf(gomock.Any(), gomock.Any()).AnyTimes()
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
