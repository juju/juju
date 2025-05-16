// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package dbreplaccessor -destination package_mock_test.go github.com/juju/juju/internal/worker/dbreplaccessor DBApp,NodeManager,TrackedDB
//go:generate go run go.uber.org/mock/mockgen -typed -package dbreplaccessor -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package dbreplaccessor -destination sql_mock_test.go database/sql/driver Driver

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	logger logger.Logger

	clock       *MockClock
	dbApp       *MockDBApp
	nodeManager *MockNodeManager

	newDBReplWorker func() (TrackedDB, error)
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.dbApp = NewMockDBApp(ctrl)
	s.nodeManager = NewMockNodeManager(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	s.newDBReplWorker = nil

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}

func (s *baseSuite) newWorkerWithDB(c *tc.C, db TrackedDB) worker.Worker {
	cfg := WorkerConfig{
		NodeManager: s.nodeManager,
		Clock:       s.clock,
		Logger:      s.logger,
		NewApp: func(driverName string) (DBApp, error) {
			return s.dbApp, nil
		},
		NewDBReplWorker: func(context.Context, DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error) {
			if s.newDBReplWorker != nil {
				return s.newDBReplWorker()
			}
			return db, nil
		},
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

func ensureStartup(c *tc.C, w *dbReplWorker) {
	select {
	case <-w.dbReplReady:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for Dqlite node start")
	}
}
