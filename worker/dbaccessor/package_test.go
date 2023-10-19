// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"testing"
	"time"

	"github.com/juju/clock"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	databasetesting "github.com/juju/juju/database/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package dbaccessor -destination package_mock_test.go github.com/juju/juju/worker/dbaccessor Logger,DBApp,NodeManager,TrackedDB,Hub,Client
//go:generate go run go.uber.org/mock/mockgen -package dbaccessor -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package dbaccessor -destination metrics_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	clock                *MockClock
	hub                  *MockHub
	timer                *MockTimer
	logger               *MockLogger
	dbApp                *MockDBApp
	client               *MockClient
	trackedDB            *MockTrackedDB
	prometheusRegisterer *MockRegisterer
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.hub = NewMockHub(ctrl)
	s.dbApp = NewMockDBApp(ctrl)
	s.client = NewMockClient(ctrl)
	s.trackedDB = NewMockTrackedDB(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Errorf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Logf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().AnyTimes()
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

func (s *baseSuite) expectWorkerRetry() {
	s.clock.EXPECT().After(10 * time.Second).AnyTimes().DoAndReturn(func(d time.Duration) <-chan time.Time {
		return clock.WallClock.After(10 * time.Millisecond)
	})
}

func (s *baseSuite) setupTimer() chan time.Time {
	s.timer.EXPECT().Stop().MinTimes(1)
	s.clock.EXPECT().NewTimer(PollInterval).Return(s.timer)

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()
	return ch
}

func (s *baseSuite) expectTick(ch chan time.Time, ticks int) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		for i := 0; i < ticks; i++ {
			ch <- time.Now()
		}
	}()
	return done
}

func (s *baseSuite) expectTimer(ticks int) <-chan struct{} {
	ch := s.setupTimer()
	return s.expectTick(ch, ticks)
}

// expectTrackedDBKill accommodates termination of the TrackedDB.
// the expectations are soft, because the worker may not have called the
// NewDBWorker function before it is killed.
func (s *baseSuite) expectTrackedDBKill() {
	s.trackedDB.EXPECT().Kill().AnyTimes()
	s.trackedDB.EXPECT().Wait().Return(nil).AnyTimes()
}

func (s *baseSuite) expectNodeStartupAndShutdown() {
	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.Client(gomock.Any()).Return(s.client, nil).MinTimes(1)
	appExp.ID().Return(uint64(666))
	appExp.Close().Return(nil)

	// The worker created in openDatabase can retry if the dbApp isn't ready
	// after it bounces.
	s.expectWorkerRetry()
}

type dbBaseSuite struct {
	databasetesting.ControllerSuite
	baseSuite
}

func ensureStartup(c *gc.C, w *dbWorker) {
	select {
	case <-w.dbReady:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("timed out waiting for Dqlite node start")
	}
}
