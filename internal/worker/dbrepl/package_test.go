// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package dbrepl -destination clock_mock_test.go github.com/juju/clock Clock,Timer

type baseSuite struct {
	jujutesting.IsolationSuite

	logger logger.Logger

	clock *MockClock
	timer *MockTimer
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
