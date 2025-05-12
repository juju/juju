// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database/app"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package dbaccessor -destination package_mock_test.go github.com/juju/juju/internal/worker/dbaccessor DBApp,NodeManager,TrackedDB,Client,ClusterConfig
//go:generate go run go.uber.org/mock/mockgen -typed -package dbaccessor -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package dbaccessor -destination metrics_mock_test.go github.com/prometheus/client_golang/prometheus Registerer
//go:generate go run go.uber.org/mock/mockgen -typed -package dbaccessor -destination controllerconfig_mock_test.go github.com/juju/juju/internal/worker/controlleragentconfig ConfigWatcher

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	logger logger.Logger

	clock                   *MockClock
	timer                   *MockTimer
	dbApp                   *MockDBApp
	client                  *MockClient
	prometheusRegisterer    *MockRegisterer
	nodeManager             *MockNodeManager
	controllerConfigWatcher *MockConfigWatcher
	clusterConfig           *MockClusterConfig
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.dbApp = NewMockDBApp(ctrl)
	s.client = NewMockClient(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)
	s.nodeManager = NewMockNodeManager(ctrl)
	s.controllerConfigWatcher = NewMockConfigWatcher(ctrl)
	s.clusterConfig = NewMockClusterConfig(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}

func (s *baseSuite) setupTimer(interval time.Duration) chan time.Time {
	s.timer.EXPECT().Stop().MinTimes(1)
	s.clock.EXPECT().NewTimer(interval).Return(s.timer)

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()
	return ch
}

func (s *baseSuite) expectTimer(ticks int) func() {
	done := make(chan struct{})

	ch := s.setupTimer(PollInterval)
	go func() {
		for i := 0; i < ticks; i++ {
			select {
			case ch <- time.Now():
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}

func (s *baseSuite) expectNoConfigChanges() {
	ch := make(chan struct{})
	s.expectConfigChanges(ch)
}

func (s *baseSuite) expectConfigChanges(ch chan struct{}) {
	exp := s.controllerConfigWatcher.EXPECT()

	exp.Changes().Return(ch).AnyTimes()
	exp.Unsubscribe().AnyTimes()
}

// expectNodeStartupAndShutdown encompasses expectations for starting the
// Dqlite app, ensuring readiness, logging and updating the node info,
// and shutting it down when the worker exits.
func (s *baseSuite) expectNodeStartupAndShutdown() {
	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.Client(gomock.Any()).Return(s.client, nil).MinTimes(1)
	appExp.ID().Return(uint64(666)).MinTimes(1)
	appExp.Address().Return("192.168.6.6:17666")
	appExp.Close().Return(nil)

	// The worker created in openDatabase can retry if the dbApp isn't ready
	// after it bounces.
	s.expectWorkerRetry()
}

func (s *baseSuite) expectWorkerRetry() {
	s.clock.EXPECT().After(10 * time.Second).AnyTimes().DoAndReturn(func(d time.Duration) <-chan time.Time {
		return clock.WallClock.After(10 * time.Millisecond)
	})
}

func (s *baseSuite) newWorkerWithDB(c *tc.C, db TrackedDB) worker.Worker {
	cfg := WorkerConfig{
		NodeManager:  s.nodeManager,
		Clock:        s.clock,
		ControllerID: "0",
		Logger:       s.logger,
		NewApp: func(string, ...app.Option) (DBApp, error) {
			return s.dbApp, nil
		},
		NewDBWorker: func(context.Context, DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error) {
			return db, nil
		},
		MetricsCollector:        &Collector{},
		ControllerConfigWatcher: s.controllerConfigWatcher,
		ClusterConfig:           s.clusterConfig,
	}

	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

type dbBaseSuite struct {
	domaintesting.ControllerSuite
	baseSuite
}

func (s *dbBaseSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
}

func (s *dbBaseSuite) TearDownTest(c *tc.C) {
	s.ControllerSuite.TearDownTest(c)
}

func ensureStartup(c *tc.C, w *dbWorker) {
	select {
	case <-w.dbReady:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for Dqlite node start")
	}
}
