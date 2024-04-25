// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservicefactory

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package providerservicefactory -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ProviderServiceFactory,ProviderServiceFactoryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package providerservicefactory -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	logger   logger.Logger
	dbGetter *MockWatchableDBGetter

	providerServiceFactory       *MockProviderServiceFactory
	providerServiceFactoryGetter *MockProviderServiceFactoryGetter
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.providerServiceFactory = NewMockProviderServiceFactory(ctrl)
	s.providerServiceFactoryGetter = NewMockProviderServiceFactoryGetter(ctrl)

	return ctrl
}
