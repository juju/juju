// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"net/http"
	"testing"

	"github.com/juju/clock"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	changestream "github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/storage"
	domaintesting "github.com/juju/juju/domain/schema/testing"
	domainservicefactory "github.com/juju/juju/domain/services"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	services "github.com/juju/juju/internal/services"
	sshimporter "github.com/juju/juju/internal/ssh/importer"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination domainservices_mock_test.go github.com/juju/juju/internal/services ControllerDomainServices,ModelDomainServices,DomainServices,DomainServicesGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination providertracker_mock_test.go github.com/juju/juju/core/providertracker Provider,ProviderFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore,ObjectStoreGetter,ModelObjectStoreGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package domainservices -destination storage_mock_test.go github.com/juju/juju/core/storage StorageRegistryGetter,ModelStorageRegistryGetter

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

	storageRegistryGetter      *MockStorageRegistryGetter
	modelStorageRegistryGetter *MockModelStorageRegistryGetter

	sshImporter *sshimporter.Importer
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

	s.storageRegistryGetter = NewMockStorageRegistryGetter(ctrl)
	s.modelStorageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)

	s.sshImporter = sshimporter.NewImporter(&http.Client{})

	return ctrl
}

// NewModelDomainServices returns a new model domain services.
// This creates a model domain services without a provider tracker. The provider
// tracker will return not supported errors for all methods.
func NewModelDomainServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	objectStore objectstore.ModelObjectStoreGetter,
	storageRegistry storage.ModelStorageRegistryGetter,
	sshImporter *sshimporter.Importer,
	clock clock.Clock,
	logger logger.Logger,
) services.ModelDomainServices {
	return domainservicefactory.NewModelFactory(
		modelUUID,
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		NoopProviderFactory{},
		objectStore,
		storageRegistry,
		sshImporter,
		clock,
		logger,
	)
}
