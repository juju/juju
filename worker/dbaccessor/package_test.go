// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"testing"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/app"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package dbaccessor -destination package_mock_test.go github.com/juju/juju/worker/dbaccessor Logger,DBApp,NodeManager,TrackedDB,Hub,Client
//go:generate go run go.uber.org/mock/mockgen -package dbaccessor -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package dbaccessor -destination metrics_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger Logger

	clock                *MockClock
	hub                  *MockHub
	timer                *MockTimer
	dbApp                *MockDBApp
	client               *MockClient
	prometheusRegisterer *MockRegisterer
	nodeManager          *MockNodeManager
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.hub = NewMockHub(ctrl)
	s.dbApp = NewMockDBApp(ctrl)
	s.client = NewMockClient(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)
	s.nodeManager = NewMockNodeManager(ctrl)

	s.logger = jujujujutesting.CheckLogger{
		Log: c,
	}

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

func (s *baseSuite) setupTimer(interval time.Duration) chan time.Time {
	s.timer.EXPECT().Stop().MinTimes(1)
	s.clock.EXPECT().NewTimer(interval).Return(s.timer)

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
	ch := s.setupTimer(PollInterval)
	return s.expectTick(ch, ticks)
}

func (s *baseSuite) expectNodeStartupAndShutdown() {
	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.Client(gomock.Any()).Return(s.client, nil).MinTimes(1)
	appExp.ID().Return(uint64(666))
	appExp.Close().Return(nil)
}

func (s *baseSuite) newWorkerWithDB(c *gc.C, db TrackedDB) worker.Worker {
	cfg := WorkerConfig{
		NodeManager:  s.nodeManager,
		Clock:        s.clock,
		Hub:          s.hub,
		ControllerID: "0",
		Logger:       s.logger,
		NewApp: func(string, ...app.Option) (DBApp, error) {
			return s.dbApp, nil
		},
		NewDBWorker: func(context.Context, DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error) {
			return db, nil
		},
		MetricsCollector: &Collector{},
	}

	w, err := NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

type dbBaseSuite struct {
	domaintesting.ControllerSuite
	baseSuite
}

func ensureStartup(c *gc.C, w *dbWorker) {
	select {
	case <-w.dbReady:
	case <-time.After(jujujujutesting.LongWait):
		c.Fatal("timed out waiting for Dqlite node start")
	}
}
