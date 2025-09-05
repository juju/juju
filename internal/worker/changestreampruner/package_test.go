// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package changestreampruner -destination stream_mock_test.go github.com/juju/juju/internal/worker/changestreampruner ChangeStreamService
//go:generate go run go.uber.org/mock/mockgen -typed -package changestreampruner -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package changestreampruner -destination worker_mock_test.go github.com/juju/worker/v4 Worker

type baseSuite struct {
	databasetesting.DqliteSuite

	changeStreamService *MockChangeStreamService
	clock               *MockClock
	timer               *MockTimer
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *baseSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.ApplyDDL(c, &domaintesting.SchemaApplier{
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

	s.changeStreamService = NewMockChangeStreamService(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

func (s *baseSuite) expectTimerRepeated(times int, done chan struct{}) {
	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer)

	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}).Times(times)
	s.timer.EXPECT().Reset(gomock.Any()).Times(times)

	// This call will block until the test is done.
	s.timer.EXPECT().Chan().DoAndReturn(func() <-chan time.Time {
		defer func() {
			if done != nil {
				close(done)
			}
		}()

		ch := make(chan time.Time, 1)
		return ch
	})

	s.timer.EXPECT().Stop().Return(true).AnyTimes()
}
