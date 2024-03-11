// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservicefactory

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domaintesting "github.com/juju/juju/domain/schema/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package providerservicefactory -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ProviderServiceFactory,ProviderServiceFactoryGetter
//go:generate go run go.uber.org/mock/mockgen -package providerservicefactory -destination logger_mock_test.go github.com/juju/juju/internal/worker/providerservicefactory Logger
//go:generate go run go.uber.org/mock/mockgen -package providerservicefactory -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	logger   *MockLogger
	dbGetter *MockWatchableDBGetter

	providerServiceFactory       *MockProviderServiceFactory
	providerServiceFactoryGetter *MockProviderServiceFactoryGetter
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.providerServiceFactory = NewMockProviderServiceFactory(ctrl)
	s.providerServiceFactoryGetter = NewMockProviderServiceFactoryGetter(ctrl)

	return ctrl
}
