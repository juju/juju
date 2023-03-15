// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"testing"
	time "time"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	dbtesting "github.com/juju/juju/database/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package stream -destination stream_mock_test.go github.com/juju/juju/worker/changestream/stream FileNotifier
//go:generate go run github.com/golang/mock/mockgen -package stream -destination logger_mock_test.go github.com/juju/juju/worker/changestream/stream Logger
//go:generate go run github.com/golang/mock/mockgen -package stream -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	dbtesting.ControllerSuite

	clock        *MockClock
	timer        *MockTimer
	logger       *MockLogger
	FileNotifier *MockFileNotifier
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.FileNotifier = NewMockFileNotifier(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(false).AnyTimes()
}

func (s *baseSuite) setupTimer() chan<- time.Time {
	s.timer.EXPECT().Stop().MinTimes(1)
	s.timer.EXPECT().Reset(gomock.Any()).AnyTimes()

	s.clock.EXPECT().NewTimer(PollInterval).Return(s.timer)

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()
	return ch
}

func (s *baseSuite) expectTick(ch chan<- time.Time, ticks int) <-chan struct{} {
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

func (s *baseSuite) expectFileNotifyWatcher() chan bool {
	ch := make(chan bool)
	s.FileNotifier.EXPECT().Changes().Return(ch, nil).MinTimes(1)
	return ch
}
