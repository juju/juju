// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/crossmodelrelation/service ControllerDBState,ModelDBState,ModelMigrationState

type baseSuite struct {
	controllerDBState *MockControllerDBState
	modelDBState      *MockModelDBState
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerDBState = NewMockControllerDBState(ctrl)
	s.modelDBState = NewMockModelDBState(ctrl)

	c.Cleanup(func() {
		s.controllerDBState = nil
		s.modelDBState = nil
	})
	return ctrl
}
