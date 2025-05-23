// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination service_mock_test.go github.com/juju/juju/internal/worker/objectstoredrainer ObjectStoreService
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination fortress_mock_test.go github.com/juju/juju/internal/worker/fortress Guard

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	service *MockObjectStoreService
	guard   *MockGuard
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockObjectStoreService(ctrl)
	s.guard = NewMockGuard(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
