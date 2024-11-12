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

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination services_mock_test.go github.com/juju/juju/apiserver/facades/client/application ExternalControllerService,NetworkService,StorageInterface,DeployFromRepository,BlockChecker,ModelConfigService,CloudService,CredentialService,MachineService,ApplicationService,PortService,StubService,Leadership,StorageService
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination legacy_mock_test.go github.com/juju/juju/apiserver/facades/client/application Backend,Application,Model,CaasBrokerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer,Resources

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	api *APIBase

	externalControllerService *MockExternalControllerService
	networkService            *MockNetworkService
	modelConfigService        *MockModelConfigService
	cloudService              *MockCloudService
	credentialService         *MockCredentialService
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

	modelInfo model.ReadOnlyModel

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
	s.cloudService = NewMockCloudService(ctrl)
	s.credentialService = NewMockCredentialService(ctrl)
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

	uuid := modeltesting.GenModelUUID(c)

	s.modelInfo = model.ReadOnlyModel{
		UUID: uuid,
	}

	return ctrl
}

func (s *baseSuite) expectAuthClient(c *gc.C) {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *baseSuite) expectHasWritePermission(c *gc.C) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelInfo.UUID.String())).Return(nil)
}

func (s *baseSuite) expectAllowBlockChange(c *gc.C) {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
}

func (s *baseSuite) expectDisallowBlockChange(c *gc.C) {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(fmt.Errorf("deploy blocked"))
}

func (s *baseSuite) newAPI(c *gc.C) {
	var err error
	s.api, err = NewAPIBase(
		s.backend,
		Services{
			ExternalControllerService: s.externalControllerService,
			NetworkService:            s.networkService,
			ModelConfigService:        s.modelConfigService,
			CloudService:              s.cloudService,
			CredentialService:         s.credentialService,
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
		func(c Charm) *state.Charm {
			panic("should not be called")
		},
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
