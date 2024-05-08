// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domaintesting "github.com/juju/juju/domain/schema/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ControllerServiceFactory,ModelServiceFactory,ServiceFactory,ServiceFactoryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination servicefactory_logger_mock_test.go github.com/juju/juju/internal/worker/servicefactory Logger
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package servicefactory -destination providertracker_mock_test.go github.com/juju/juju/core/providertracker Provider,ProviderFactory

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	logger    *MockLogger
	dbDeleter *MockDBDeleter
	dbGetter  *MockWatchableDBGetter

	serviceFactoryGetter     *MockServiceFactoryGetter
	controllerServiceFactory *MockControllerServiceFactory
	modelServiceFactory      *MockModelServiceFactory

	provider        *MockProvider
	providerFactory *MockProviderFactory
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.dbDeleter = NewMockDBDeleter(ctrl)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.serviceFactoryGetter = NewMockServiceFactoryGetter(ctrl)
	s.controllerServiceFactory = NewMockControllerServiceFactory(ctrl)
	s.modelServiceFactory = NewMockModelServiceFactory(ctrl)

	s.provider = NewMockProvider(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)

	return ctrl
}
