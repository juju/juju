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

//go:generate go run github.com/golang/mock/mockgen -package changestream -destination stream_mock_test.go github.com/juju/juju/worker/changestream ChangeStream,DBGetter,EventQueueWorker,EventQueue,FileNotifyWatcher
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
	fileNotifyWatcher *MockFileNotifyWatcher
	eventQueue        *MockEventQueue
	eventQueueWorker  *MockEventQueueWorker
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.fileNotifyWatcher = NewMockFileNotifyWatcher(ctrl)
	s.eventQueue = NewMockEventQueue(ctrl)
	s.eventQueueWorker = NewMockEventQueueWorker(ctrl)

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
