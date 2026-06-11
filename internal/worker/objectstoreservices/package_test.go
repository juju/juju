// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreservices

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run github.com/canonical/gomock/mockgen -package objectstoreservices -destination servicefactory_mock_test.go github.com/juju/juju/internal/services ObjectStoreServices,ObjectStoreServicesGetter
//go:generate go run github.com/canonical/gomock/mockgen -package objectstoreservices -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter

type baseSuite struct {
	domaintesting.ControllerSuite

	logger   logger.Logger
	dbGetter *MockWatchableDBGetter

	objectStoreServices       *MockObjectStoreServices
	objectStoreServicesGetter *MockObjectStoreServicesGetter
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.objectStoreServices = NewMockObjectStoreServices(ctrl)
	s.objectStoreServicesGetter = NewMockObjectStoreServicesGetter(ctrl)

	return ctrl
}
