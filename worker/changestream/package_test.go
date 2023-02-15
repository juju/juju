// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"testing"
	time "time"

	"github.com/golang/mock/gomock"
	dbtesting "github.com/juju/juju/database/testing"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package changestream -destination stream_mock_test.go github.com/juju/juju/worker/changestream ChangeStream,DBGetter,DBStream,FileNotifier,FileNotifyWatcher
//go:generate go run github.com/golang/mock/mockgen -package changestream -destination logger_mock_test.go github.com/juju/juju/worker/changestream Logger
//go:generate go run github.com/golang/mock/mockgen -package changestream -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	dbtesting.ControllerSuite

	dbGetter          *MockDBGetter
	clock             *MockClock
	timer             *MockTimer
	logger            *MockLogger
	dbStream          *MockDBStream
	fileNotifyWatcher *MockFileNotifyWatcher
	FileNotifier      *MockFileNotifier
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.dbStream = NewMockDBStream(ctrl)
	s.fileNotifyWatcher = NewMockFileNotifyWatcher(ctrl)
	s.FileNotifier = NewMockFileNotifier(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Errorf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().IsTraceEnabled().Return(false).AnyTimes()
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}

func (s *baseSuite) expectTimer(ticks int) <-chan struct{} {
	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()
	s.timer.EXPECT().Stop().MinTimes(1)
	s.timer.EXPECT().Reset(gomock.Any()).AnyTimes()

	s.clock.EXPECT().NewTimer(PollInterval).Return(s.timer)

	done := make(chan struct{})
	go func() {
		close(done)

		for i := 0; i < ticks; i++ {
			ch <- time.Now()
		}
	}()
	return done
}

func (s *baseSuite) expectFileNotifyWatcher() {
	ch := make(chan bool)
	s.FileNotifier.EXPECT().Changes().Return(ch, nil).MinTimes(1)
}
