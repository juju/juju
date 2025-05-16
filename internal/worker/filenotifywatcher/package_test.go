// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	stdtesting "testing"
	time "time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package filenotifywatcher -destination watcher_mock_test.go github.com/juju/juju/internal/worker/filenotifywatcher FileNotifyWatcher,FileWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package filenotifywatcher -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

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
