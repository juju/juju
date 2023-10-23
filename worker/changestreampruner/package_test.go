// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package changestreampruner -destination stream_mock_test.go github.com/juju/juju/worker/changestreampruner DBGetter,Logger
//go:generate go run go.uber.org/mock/mockgen -package changestreampruner -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package changestreampruner -destination worker_mock_test.go github.com/juju/worker/v3 Worker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	databasetesting.DqliteSuite

	dbGetter *MockDBGetter
	clock    *MockClock
	timer    *MockTimer
	logger   *MockLogger
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *baseSuite) SetUpTest(c *gc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &domaintesting.SchemaApplier{
		Schema: schema.ControllerDDL(),
	})
}

// ApplyDDLForRunner is responsible for applying the controller schema to the
// given database.
func (s *baseSuite) ApplyDDLForRunner(c *gc.C, runner coredatabase.TxnRunner) {
	s.DqliteSuite.ApplyDDLForRunner(c, &domaintesting.SchemaApplier{
		Schema: schema.ControllerDDL(),
	}, runner)
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)

	return ctrl
}

func (s *baseSuite) expectControllerDBGet() {
	s.dbGetter.EXPECT().GetDB(coredatabase.ControllerNS).Return(s.TxnRunner(), nil).Times(2)
}

func (s *baseSuite) expectDBGet(namespace string, txnRunner coredatabase.TxnRunner) {
	s.dbGetter.EXPECT().GetDB(namespace).Return(txnRunner, nil)
}

func (s *baseSuite) expectAnyLogs(c *gc.C) {
	s.logger.EXPECT().Errorf(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any(), gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).Do(c.Logf).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(false).AnyTimes()
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}
