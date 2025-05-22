// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservices

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package providerservices -destination servicefactory_mock_test.go github.com/juju/juju/internal/services ProviderServices,ProviderServicesGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package providerservices -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter

type baseSuite struct {
	domaintesting.ControllerSuite

	logger   logger.Logger
	dbGetter *MockWatchableDBGetter

	providerServices       *MockProviderServices
	providerServicesGetter *MockProviderServicesGetter
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.providerServices = NewMockProviderServices(ctrl)
	s.providerServicesGetter = NewMockProviderServicesGetter(ctrl)

	return ctrl
}
