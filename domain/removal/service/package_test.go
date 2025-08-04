// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/removal/service ControllerDBState,ModelDBState,Provider
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leadership_mock_test.go github.com/juju/juju/core/leadership Revoker
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination clock_mock_test.go github.com/juju/clock Clock

type baseSuite struct {
	testhelpers.IsolationSuite

	modelUUID model.UUID

	controllerState *MockControllerDBState
	modelState      *MockModelDBState
	clock           *MockClock
	revoker         *MockRevoker
	provider        *MockProvider
}

func (s *baseSuite) newService(c *tc.C) *Service {
	return &Service{
		controllerState:   s.controllerState,
		modelState:        s.modelState,
		leadershipRevoker: s.revoker,
		modelUUID:         s.modelUUID,
		provider: func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		clock:  s.clock,
		logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelUUID = modeltesting.GenModelUUID(c)

	s.controllerState = NewMockControllerDBState(ctrl)
	s.modelState = NewMockModelDBState(ctrl)
	s.clock = NewMockClock(ctrl)
	s.revoker = NewMockRevoker(ctrl)
	s.provider = NewMockProvider(ctrl)

	c.Cleanup(func() {
		s.modelState = nil
		s.clock = nil
		s.revoker = nil
		s.provider = nil
	})

	return ctrl
}
