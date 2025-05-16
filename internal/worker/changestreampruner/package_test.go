// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package changestreampruner -destination stream_mock_test.go github.com/juju/juju/internal/worker/changestreampruner DBGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package changestreampruner -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package changestreampruner -destination worker_mock_test.go github.com/juju/worker/v4 Worker

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	databasetesting.DqliteSuite

	dbGetter *MockDBGetter
	clock    *MockClock
	timer    *MockTimer
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *baseSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &domaintesting.SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	})
}

// ApplyDDLForRunner is responsible for applying the controller schema to the
// given database.
func (s *baseSuite) ApplyDDLForRunner(c *tc.C, runner coredatabase.TxnRunner) {
	s.DqliteSuite.ApplyDDLForRunner(c, &domaintesting.SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	}, runner)
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)

	return ctrl
}

func (s *baseSuite) expectControllerDBGet() {
	s.dbGetter.EXPECT().GetDB(coredatabase.ControllerNS).Return(s.TxnRunner(), nil).Times(2)
}

func (s *baseSuite) expectDBGet(namespace string, txnRunner coredatabase.TxnRunner) {
	s.expectDBGetTimes(namespace, txnRunner, 1)
}

func (s *baseSuite) expectDBGetTimes(namespace string, txnRunner coredatabase.TxnRunner, amount int) {
	s.dbGetter.EXPECT().GetDB(namespace).Return(txnRunner, nil).Times(amount)
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}
