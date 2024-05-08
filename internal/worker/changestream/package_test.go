// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"testing"
	"time"

	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domaintesting "github.com/juju/juju/domain/schema/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination stream_mock_test.go github.com/juju/juju/internal/worker/changestream DBGetter,Logger,WatchableDBWorker,FileNotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination source_mock_test.go github.com/juju/juju/core/changestream EventSource
//go:generate go run go.uber.org/mock/mockgen -typed -package changestream -destination metrics_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	dbGetter             *MockDBGetter
	clock                *MockClock
	timer                *MockTimer
	logger               *MockLogger
	fileNotifyWatcher    *MockFileNotifyWatcher
	eventSource          *MockEventSource
	prometheusRegisterer *MockRegisterer
	watchableDBWorker    *MockWatchableDBWorker
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockDBGetter(ctrl)
	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.fileNotifyWatcher = NewMockFileNotifyWatcher(ctrl)
	s.eventSource = NewMockEventSource(ctrl)
	s.prometheusRegisterer = NewMockRegisterer(ctrl)
	s.watchableDBWorker = NewMockWatchableDBWorker(ctrl)

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
