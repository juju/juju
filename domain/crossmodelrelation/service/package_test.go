// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/domain"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/crossmodelrelation/service ControllerState,ModelState,ModelMigrationState,ModelRelationNetworkState

type baseSuite struct {
	controllerState *MockControllerState
	modelState      *MockModelState
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerState = NewMockControllerState(ctrl)
	s.modelState = NewMockModelState(ctrl)

	c.Cleanup(func() {
		s.controllerState = nil
		s.modelState = nil
	})
	return ctrl
}

func (s *baseSuite) service(c *tc.C) *Service {
	return &Service{
		controllerState: s.controllerState,
		modelState:      s.modelState,
		statusHistory:   domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
