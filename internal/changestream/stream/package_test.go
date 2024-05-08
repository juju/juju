// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"testing"
	time "time"

	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/schema"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package stream -destination stream_mock_test.go github.com/juju/juju/internal/changestream/stream FileNotifier
//go:generate go run go.uber.org/mock/mockgen -typed -package stream -destination logger_mock_test.go github.com/juju/juju/internal/changestream/stream Logger
//go:generate go run go.uber.org/mock/mockgen -typed -package stream -destination metrics_mock_test.go github.com/juju/juju/internal/changestream/stream MetricsCollector
//go:generate go run go.uber.org/mock/mockgen -typed -package stream -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	databasetesting.DqliteSuite

	clock        *MockClock
	timer        *MockTimer
	logger       *MockLogger
	metrics      *MockMetricsCollector
	FileNotifier *MockFileNotifier
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *baseSuite) SetUpTest(c *gc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &domaintesting.SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	})
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.metrics = NewMockMetricsCollector(ctrl)
	s.FileNotifier = NewMockFileNotifier(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Errorf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(false).AnyTimes()
}

func (s *baseSuite) expectTimer() chan<- time.Time {
	ch := make(chan time.Time)

	s.clock.EXPECT().NewTimer(gomock.Any()).Return(s.timer).AnyTimes()
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()
	s.timer.EXPECT().Stop().MinTimes(1)

	return ch
}

func (s *baseSuite) expectAfter() chan<- time.Time {
	ch := make(chan time.Time)

	s.clock.EXPECT().After(gomock.Any()).Return(ch)

	return ch
}

func (s *baseSuite) expectAfterAnyTimes() {
	s.clock.EXPECT().After(defaultWaitTermTimeout).Return(make(chan time.Time)).AnyTimes()
}

func (s *baseSuite) expectBackoffAnyTimes(done chan struct{}) {
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		go func() {
			select {
			case ch <- time.Now():
			case <-done:
				return
			}
		}()
		return ch
	}).AnyTimes()
}

func (s *baseSuite) expectFileNotifyWatcher() chan bool {
	ch := make(chan bool)
	s.FileNotifier.EXPECT().Changes().Return(ch, nil).MinTimes(1)
	return ch
}

func (s *baseSuite) expectMetrics() {
	s.metrics.EXPECT().ChangesRequestDurationObserve(gomock.Any()).AnyTimes()
	s.metrics.EXPECT().ChangesCountObserve(gomock.Any()).AnyTimes()
	s.metrics.EXPECT().WatermarkInsertsInc().AnyTimes()
	s.metrics.EXPECT().WatermarkRetriesInc().AnyTimes()
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().AnyTimes()
}
