// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package undertaker -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run go.uber.org/mock/mockgen -typed -package undertaker -destination package_mock_test.go github.com/juju/juju/internal/worker/undertaker ControllerModelService

type baseSuite struct {
	testhelpers.IsolationSuite

	dbDeleter              *MockDBDeleter
	controllerModelService *MockControllerModelService

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbDeleter = NewMockDBDeleter(ctrl)
	s.controllerModelService = NewMockControllerModelService(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	c.Cleanup(func() {
		s.dbDeleter = nil
		s.controllerModelService = nil
		s.logger = nil
	})

	return ctrl
}
