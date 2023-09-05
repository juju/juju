// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"testing"
	time "time"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domaintesting "github.com/juju/juju/domain/schema/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package stream -destination stream_mock_test.go github.com/juju/juju/internal/changestream/stream FileNotifier
//go:generate go run go.uber.org/mock/mockgen -package stream -destination logger_mock_test.go github.com/juju/juju/internal/changestream/stream Logger
//go:generate go run go.uber.org/mock/mockgen -package stream -destination metrics_mock_test.go github.com/juju/juju/internal/changestream/stream MetricsCollector
//go:generate go run go.uber.org/mock/mockgen -package stream -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	clock        *MockClock
	timer        *MockTimer
	logger       *MockLogger
	metrics      *MockMetricsCollector
	FileNotifier *MockFileNotifier
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
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		go func() {
			ch <- time.Now()
		}()
		return ch
	}).AnyTimes()
}

func (s *baseSuite) expectFileNotifyWatcher() chan bool {
	ch := make(chan bool)
	s.FileNotifier.EXPECT().Changes().Return(ch, nil).MinTimes(1)
	return ch
}
