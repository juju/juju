// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	domaintesting "github.com/juju/juju/domain/schema/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination stream_mock_test.go github.com/juju/juju/internal/worker/changestream DBGetter,WatchableDBWorker,FileNotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination source_mock_test.go github.com/juju/juju/core/changestream EventSource
//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination metrics_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	dbGetter             *MockDBGetter
	clock                *MockClock
	timer                *MockTimer
	fileNotifyWatcher    *MockFileNotifyWatcher
	eventSource          *MockEventSource
	prometheusRegisterer *MockRegisterer
	watchableDBWorker    *MockWatchableDBWorker
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.fileNotifyWatcher = NewMockFileNotifyWatcher(ctrl)
	s.eventSource = NewMockEventSource(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)
	s.watchableDBWorker = NewMockWatchableDBWorker(ctrl)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
}
