// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/removal/service State
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination clock_mock_test.go github.com/juju/clock Clock

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

	state *MockState
	clock *MockClock
}

func (s *baseSuite) newService(c *tc.C) *Service {
	return &Service{
		st:     s.state,
		clock:  s.clock,
		logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.clock = NewMockClock(ctrl)

	return ctrl
}
