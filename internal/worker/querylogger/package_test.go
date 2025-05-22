// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package querylogger -destination logger_mock_test.go github.com/juju/juju/core/logger Logger
//go:generate go run go.uber.org/mock/mockgen -typed -package querylogger -destination clock_mock_test.go github.com/juju/clock Clock,Timer


type baseSuite struct {
	testhelpers.IsolationSuite

	clock  *MockClock
	timer  *MockTimer
	logger *MockLogger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.logger.EXPECT().Helper().AnyTimes()

	return ctrl
}
