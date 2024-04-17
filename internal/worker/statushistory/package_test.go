// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"testing"
	"time"

	jujutesting "github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package statushistory -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package statushistory -destination logsink_mock_test.go github.com/juju/juju/core/logger ModelLogger,LoggerCloser

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logSink *MockModelLogger
	logger  *MockLoggerCloser
	clock   *MockClock
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logSink = NewMockModelLogger(ctrl)
	s.logger = NewMockLoggerCloser(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *baseSuite) expectClock(now time.Time) {
	s.clock.EXPECT().Now().Return(now).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}
