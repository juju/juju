// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination services_mock_test.go github.com/juju/juju/apiserver/facades/client/application NetworkService,StorageInterface,DeployFromRepository,BlockChecker,ModelConfigService,MachineService,ApplicationService,ResolveService,PortService,Leadership,StorageService,RelationService,ResourceService,RemovalService
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination legacy_mock_test.go github.com/juju/juju/apiserver/facades/client/application Backend,CaasBrokerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm,CharmMeta
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination core_charm_mock_test.go github.com/juju/juju/core/charm Repository,RepositoryFactory

type baseSuite struct {
	testhelpers.IsolationSuite

	api *APIBase

	applicationService *MockApplicationService
	resolveService     *MockResolveService
	machineService     *MockMachineService
	modelConfigService *MockModelConfigService
	networkService     *MockNetworkService
	portService        *MockPortService
	resourceService    *MockResourceService
	storageService     *MockStorageService
	relationService    *MockRelationService
	removalService     *MockRemovalService
	storageAccess      *MockStorageInterface
	authorizer         *MockAuthorizer
	blockChecker       *MockBlockChecker
	leadershipReader   *MockLeadership
	deployFromRepo     *MockDeployFromRepository
	objectStore        *MockObjectStore

	charmRepository        *MockRepository
	charmRepositoryFactory *MockRepositoryFactory

	modelUUID model.UUID
	modelType model.ModelType

	// Legacy types that we're transitioning away from.
	backend           *MockBackend
	deployApplication DeployApplicationFunc
	providerRegistry  *MockProviderRegistry
	caasBroker        *MockCaasBrokerInterface
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.resolveService = NewMockResolveService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.portService = NewMockPortService(ctrl)
	s.resourceService = NewMockResourceService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)

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

func (s *baseSuite) newIAASAPI(c *tc.C) {
	s.newAPI(c, model.IAAS)
}

func (s *baseSuite) newCAASAPI(c *tc.C) {
	s.newAPI(c, model.CAAS)
}

func (s *baseSuite) newAPI(c *tc.C, modelType model.ModelType) {
	s.deployApplication = DeployApplication
	s.modelType = modelType
	var err error
	s.api, err = NewAPIBase(
		s.backend,
		Services{
			NetworkService:     s.networkService,
			ModelConfigService: s.modelConfigService,
			MachineService:     s.machineService,
			ApplicationService: s.applicationService,
			ResolveService:     s.resolveService,
			PortService:        s.portService,
			ResourceService:    s.resourceService,
			StorageService:     s.storageService,
			RelationService:    s.relationService,
			RemovalService:     s.removalService,
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
		s.caasBroker,
		s.objectStore,
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)
	c.Assert(err, tc.ErrorIsNil)
}
