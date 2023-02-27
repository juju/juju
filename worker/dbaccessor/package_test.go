// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"testing"
	time "time"

	"github.com/golang/mock/gomock"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -source worker.go -package dbaccessor -destination package_mock_test.go github.com/juju/juju/worker/dbaccessor Logger,DBApp,NodeManager,TrackedDB
//go:generate go run github.com/golang/mock/mockgen -package dbaccessor -destination logger_mock_test.go github.com/juju/juju/worker/dbaccessor Logger
//go:generate go run github.com/golang/mock/mockgen -package dbaccessor -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	clock     *MockClock
	timer     *MockTimer
	logger    *MockLogger
	dbApp     *MockDBApp
	trackedDB *MockTrackedDB
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.dbApp = NewMockDBApp(ctrl)
	s.trackedDB = NewMockTrackedDB(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Errorf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Logf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().AnyTimes()
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

func (s *baseSuite) setupTimer() chan time.Time {
	s.timer.EXPECT().Stop().MinTimes(1)
	s.timer.EXPECT().Reset(gomock.Any()).AnyTimes()

	s.clock.EXPECT().NewTimer(PollInterval).Return(s.timer)

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
	ch := s.setupTimer()
	return s.expectTick(ch, ticks)
}

func (s *baseSuite) expectTrackedDB(c *gc.C) chan struct{} {
	done := make(chan struct{})

	s.trackedDB.EXPECT().Kill().AnyTimes()
	s.trackedDB.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(jujutesting.LongWait):
			c.Fatal("timed out waiting for Wait to be called")
		}
		return nil
	})

	return done
}
