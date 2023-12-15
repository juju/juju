// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"testing"
	time "time"

	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package filenotifywatcher -destination watcher_mock_test.go github.com/juju/juju/internal/worker/filenotifywatcher FileNotifyWatcher,FileWatcher
//go:generate go run go.uber.org/mock/mockgen -package filenotifywatcher -destination logger_mock_test.go github.com/juju/juju/internal/worker/filenotifywatcher Logger
//go:generate go run go.uber.org/mock/mockgen -package filenotifywatcher -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	clock   *MockClock
	timer   *MockTimer
	logger  *MockLogger
	watcher *MockFileWatcher
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.watcher = NewMockFileWatcher(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyLogs() {
	s.logger.EXPECT().Errorf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Warningf(gomock.Any()).AnyTimes()
	s.logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any()).AnyTimes()
}

func (s *baseSuite) expectAnyClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}
