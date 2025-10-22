// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination services_mock_test.go github.com/juju/juju/apiserver/facades/client/application NetworkService,DeployFromRepository,BlockChecker,ModelConfigService,MachineService,ApplicationService,ResolveService,PortService,Leadership,StorageService,RelationService,ResourceService,RemovalService,ExternalControllerService,CrossModelRelationService,StatusService
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination legacy_mock_test.go github.com/juju/juju/apiserver/facades/client/application CaasBrokerInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm,CharmMeta
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination core_charm_mock_test.go github.com/juju/juju/core/charm Repository,RepositoryFactory

type baseSuite struct {
	testhelpers.IsolationSuite

	api *APIBase

	applicationService        *MockApplicationService
	authorizer                *MockAuthorizer
	blockChecker              *MockBlockChecker
	crossModelRelationService *MockCrossModelRelationService
	deployFromRepo            *MockDeployFromRepository
	externalControllerService *MockExternalControllerService
	leadershipReader          *MockLeadership
	machineService            *MockMachineService
	modelConfigService        *MockModelConfigService
	networkService            *MockNetworkService
	objectStore               *MockObjectStore
	portService               *MockPortService
	relationService           *MockRelationService
	removalService            *MockRemovalService
	resolveService            *MockResolveService
	resourceService           *MockResourceService
	statusService             *MockStatusService
	storageService            *MockStorageService

	charmRepository        *MockRepository
	charmRepositoryFactory *MockRepositoryFactory

	controllerUUID string
	modelUUID      model.UUID
	modelType      model.ModelType

	// Legacy types that we're transitioning away from.
	deployApplication DeployApplicationFunc
	caasBroker        *MockCaasBrokerInterface
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.crossModelRelationService = NewMockCrossModelRelationService(ctrl)
	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.portService = NewMockPortService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.resolveService = NewMockResolveService(ctrl)
	s.resourceService = NewMockResourceService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.storageService = NewMockStorageService(ctrl)

	s.authorizer = NewMockAuthorizer(ctrl)
	s.blockChecker = NewMockBlockChecker(ctrl)
	s.leadershipReader = NewMockLeadership(ctrl)
	s.deployFromRepo = NewMockDeployFromRepository(ctrl)

	s.charmRepository = NewMockRepository(ctrl)
	s.charmRepositoryFactory = NewMockRepositoryFactory(ctrl)

	s.controllerUUID = tc.Must(c, uuid.NewUUID).String()
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
		Services{
			ApplicationService:        s.applicationService,
			CrossModelRelationService: s.crossModelRelationService,
			ExternalControllerService: s.externalControllerService,
			MachineService:            s.machineService,
			ModelConfigService:        s.modelConfigService,
			NetworkService:            s.networkService,
			PortService:               s.portService,
			RelationService:           s.relationService,
			RemovalService:            s.removalService,
			ResolveService:            s.resolveService,
			ResourceService:           s.resourceService,
			StatusService:             s.statusService,
			StorageService:            s.storageService,
		},
		s.authorizer,
		s.blockChecker,
		s.controllerUUID,
		s.modelUUID,
		s.modelType,
		s.leadershipReader,
		s.deployFromRepo,
		s.deployApplication,
		s.caasBroker,
		s.objectStore,
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
