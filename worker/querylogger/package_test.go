// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

import (
	"testing"

	"github.com/golang/mock/gomock"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package querylogger -destination package_mock_test.go github.com/juju/juju/worker/querylogger Logger
//go:generate go run github.com/golang/mock/mockgen -package querylogger -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	clock  *MockClock
	timer  *MockTimer
	logger *MockLogger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.logger = NewMockLogger(ctrl)

	return ctrl
}
