// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	databasetesting "github.com/juju/juju/database/testing"
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
	trackedDB            *MockTrackedDB
	prometheusRegisterer *MockRegisterer
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.hub = NewMockHub(ctrl)
	s.dbApp = NewMockDBApp(ctrl)
	s.client = NewMockClient(ctrl)
	s.trackedDB = NewMockTrackedDB(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)

	s.logger = jujujujutesting.CheckLogger{
		Log: c,
	}

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

func (s *baseSuite) expectAnyAfter() {
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
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

// expectTrackedDBKill accommodates termination of the TrackedDB.
// the expectations are soft, because the worker may not have called the
// NewDBWorker function before it is killed.
func (s *baseSuite) expectTrackedDBKill() {
	s.trackedDB.EXPECT().Kill().AnyTimes()
	s.trackedDB.EXPECT().Wait().Return(nil).AnyTimes()
}

type dbBaseSuite struct {
	databasetesting.ControllerSuite
	baseSuite
}

type workerTrackedDB struct {
	tomb tomb.Tomb
	db   coredatabase.TxnRunner
}

func newWorkerTrackedDB(db coredatabase.TxnRunner) *workerTrackedDB {
	w := &workerTrackedDB{
		db: db,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *workerTrackedDB) loop() error {
	<-w.tomb.Dying()
	return tomb.ErrDying
}

func (w *workerTrackedDB) Kill() {
	w.tomb.Kill(nil)
}

func (w *workerTrackedDB) Wait() error {
	return w.tomb.Wait()
}

func (w *workerTrackedDB) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return w.db.Txn(ctx, fn)
}

func (w *workerTrackedDB) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return w.db.StdTxn(ctx, fn)
}
