// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"testing"

	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination services_mock_test.go github.com/juju/juju/apiserver/facades/client/application ExternalControllerService,NetworkService,StorageInterface,DeployFromRepository,BlockChecker,ModelConfigService,MachineService,ApplicationService,PortService,StubService,Leadership,StorageService
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination legacy_mock_test.go github.com/juju/juju/apiserver/facades/client/application Backend,Application,Model,CaasBrokerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer,Resources
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm,CharmMeta
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination core_charm_mock_test.go github.com/juju/juju/core/charm Repository,RepositoryFactory

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	api *APIBase

	externalControllerService *MockExternalControllerService
	networkService            *MockNetworkService
	modelConfigService        *MockModelConfigService
	machineService            *MockMachineService
	applicationService        *MockApplicationService
	portService               *MockPortService
	storageService            *MockStorageService
	stubService               *MockStubService

	storageAccess    *MockStorageInterface
	authorizer       *MockAuthorizer
	blockChecker     *MockBlockChecker
	leadershipReader *MockLeadership
	deployFromRepo   *MockDeployFromRepository
	objectStore      *MockObjectStore

	charmRepository        *MockRepository
	charmRepositoryFactory *MockRepositoryFactory

	modelUUID model.UUID
	modelInfo model.ModelInfo

	// Legacy types that we're transitioning away from.
	backend           *MockBackend
	model             *MockModel
	deployApplication DeployApplicationFunc
	providerRegistry  *MockProviderRegistry
	resources         *MockResources
	caasBroker        *MockCaasBrokerInterface
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.portService = NewMockPortService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.stubService = NewMockStubService(ctrl)

	s.storageAccess = NewMockStorageInterface(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.blockChecker = NewMockBlockChecker(ctrl)
	s.leadershipReader = NewMockLeadership(ctrl)
	s.deployFromRepo = NewMockDeployFromRepository(ctrl)

	s.backend = NewMockBackend(ctrl)
	s.model = NewMockModel(ctrl)
	s.providerRegistry = NewMockProviderRegistry(ctrl)

	s.charmRepository = NewMockRepository(ctrl)
	s.charmRepositoryFactory = NewMockRepositoryFactory(ctrl)

	s.modelUUID = modeltesting.GenModelUUID(c)

	return ctrl
}

func (s *baseSuite) expectAuthClient(c *gc.C) {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *baseSuite) expectHasWritePermission(c *gc.C) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String())).Return(nil)
}

func (s *baseSuite) expectHasIncorrectPermission(c *gc.C) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), names.NewModelTag(s.modelUUID.String())).Return(apiservererrors.ErrPerm)
}

func (s *baseSuite) expectAnyPermissions(c *gc.C) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

func (s *baseSuite) expectAllowBlockChange(c *gc.C) {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
}

func (s *baseSuite) expectDisallowBlockChange(c *gc.C) {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(fmt.Errorf("blocked"))
}

func (s *baseSuite) expectDisallowBlockRemoval(c *gc.C) {
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(fmt.Errorf("blocked"))
}

func (s *baseSuite) expectAnyChangeOrRemoval(c *gc.C) {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil).AnyTimes()
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil).AnyTimes()
}

func (s *baseSuite) newIAASAPI(c *gc.C) {
	s.newAPI(c, model.IAAS)
}

func (s *baseSuite) newCAASAPI(c *gc.C) {
	s.newAPI(c, model.CAAS)
}

func (s *baseSuite) newAPI(c *gc.C, modelType model.ModelType) {
	s.modelInfo = model.ModelInfo{
		UUID: s.modelUUID,
		Type: modelType,
	}
	s.deployApplication = DeployApplication

	var err error
	s.api, err = NewAPIBase(
		s.backend,
		Services{
			ExternalControllerService: s.externalControllerService,
			NetworkService:            s.networkService,
			ModelConfigService:        s.modelConfigService,
			MachineService:            s.machineService,
			ApplicationService:        s.applicationService,
			PortService:               s.portService,
			StorageService:            s.storageService,
			StubService:               s.stubService,
		},
		s.storageAccess,
		s.authorizer,
		s.blockChecker,
		s.model,
		s.modelInfo,
		s.leadershipReader,
		s.deployFromRepo,
		s.deployApplication,
		s.providerRegistry,
		s.resources,
		s.caasBroker,
		s.objectStore,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
}
