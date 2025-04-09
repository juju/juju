// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/names/v6"
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

type baseSuite struct {
	jujutesting.IsolationSuite

	api *APIBase

	applicationService        *MockApplicationService
	externalControllerService *MockExternalControllerService
	machineService            *MockMachineService
	modelConfigService        *MockModelConfigService
	networkService            *MockNetworkService
	portService               *MockPortService
	resourceService           *MockResourceService
	storageService            *MockStorageService
	relationService           *MockRelationService
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
	modelType model.ModelType

	// Legacy types that we're transitioning away from.
	backend           *MockBackend
	deployApplication DeployApplicationFunc
	providerRegistry  *MockProviderRegistry
	resources         *MockResources
	caasBroker        *MockCaasBrokerInterface
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.portService = NewMockPortService(ctrl)
	s.resourceService = NewMockResourceService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.stubService = NewMockStubService(ctrl)

	s.storageAccess = NewMockStorageInterface(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.blockChecker = NewMockBlockChecker(ctrl)
	s.leadershipReader = NewMockLeadership(ctrl)
	s.deployFromRepo = NewMockDeployFromRepository(ctrl)

	s.backend = NewMockBackend(ctrl)
	s.providerRegistry = NewMockProviderRegistry(ctrl)

	s.charmRepository = NewMockRepository(ctrl)
	s.charmRepositoryFactory = NewMockRepositoryFactory(ctrl)

	s.modelUUID = modeltesting.GenModelUUID(c)

	return ctrl
}

func (s *baseSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *baseSuite) expectHasWritePermission() {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String())).Return(nil)
}

func (s *baseSuite) expectHasIncorrectPermission() {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), names.NewModelTag(s.modelUUID.String())).Return(apiservererrors.ErrPerm)
}

func (s *baseSuite) expectAnyPermissions() {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

func (s *baseSuite) expectAllowBlockChange() {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
}

func (s *baseSuite) expectDisallowBlockChange() {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(fmt.Errorf("blocked"))
}

func (s *baseSuite) expectDisallowBlockRemoval() {
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(fmt.Errorf("blocked"))
}

func (s *baseSuite) expectAnyChangeOrRemoval() {
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
	s.deployApplication = DeployApplication
	s.modelType = modelType
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
			ResourceService:           s.resourceService,
			StorageService:            s.storageService,
			RelationService:           s.relationService,
			StubService:               s.stubService,
		},
		s.storageAccess,
		s.authorizer,
		s.blockChecker,
		s.modelUUID,
		s.modelType,
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
