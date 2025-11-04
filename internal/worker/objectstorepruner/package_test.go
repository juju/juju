// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorepruner

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstorepruner -destination service_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstorepruner -destination fortress_mock_test.go github.com/juju/juju/internal/worker/objectstorepruner ObjectStoreService

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	objectStore        *MockObjectStore
	objectStoreService *MockObjectStoreService
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreService = NewMockObjectStoreService(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
