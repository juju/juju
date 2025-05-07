// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"testing"
	time "time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package filenotifywatcher -destination watcher_mock_test.go github.com/juju/juju/internal/worker/filenotifywatcher FileNotifyWatcher,FileWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package filenotifywatcher -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	clock   *MockClock
	timer   *MockTimer
	watcher *MockFileWatcher
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.watcher = NewMockFileWatcher(ctrl)

	return ctrl
}

func (s *baseSuite) expectAnyClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}
