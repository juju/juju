// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"testing"

	"github.com/juju/clock"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination domainservices_mock_test.go github.com/juju/juju/internal/services ControllerDomainServices,ModelDomainServices,DomainServices,DomainServicesGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination providertracker_mock_test.go github.com/juju/juju/core/providertracker Provider,ProviderFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore,ObjectStoreGetter,ModelObjectStoreGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	logger    logger.Logger
	clock     clock.Clock
	dbDeleter *MockDBDeleter
	dbGetter  *MockWatchableDBGetter

	domainServicesGetter     *MockDomainServicesGetter
	controllerDomainServices *MockControllerDomainServices
	modelDomainServices      *MockModelDomainServices

	provider        *MockProvider
	providerFactory *MockProviderFactory

	objectStore            *MockObjectStore
	objectStoreGetter      *MockObjectStoreGetter
	modelObjectStoreGetter *MockModelObjectStoreGetter
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = loggertesting.WrapCheckLog(c)
	s.clock = clock.WallClock
	s.dbDeleter = NewMockDBDeleter(ctrl)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.controllerDomainServices = NewMockControllerDomainServices(ctrl)
	s.modelDomainServices = NewMockModelDomainServices(ctrl)

	s.provider = NewMockProvider(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)

	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.modelObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)

	return ctrl
}
