// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination addressfinder_mock_test.go github.com/juju/juju/environs InstanceLister
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination providertracker_mock_test.go github.com/juju/juju/core/providertracker ProviderFactory
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination caas_broker_mock_test.go github.com/juju/juju/caas ServiceManager
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination instance_mock_test.go github.com/juju/juju/environs/instances Instance
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination storage_mock_test.go github.com/juju/juju/core/storage StorageRegistryGetter
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Unlocker
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/worker/bootstrap AgentBinaryStore,ControllerConfigService,FlagService,ObjectStoreGetter,HTTPClient,CloudService,StorageService,ApplicationService,ModelConfigService,NetworkService,UserService,BakeryConfigService,KeyManagerService,MachineService,AgentPasswordService,ControllerNodeService,ModelInfoService
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination http_client_mock_test.go github.com/juju/juju/core/http HTTPClientGetter
//go:generate go run github.com/canonical/gomock/mockgen -package bootstrap -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServices

type baseSuite struct {
	testhelpers.IsolationSuite

	dataDir string

	controllerAgentBinaryStore *MockAgentBinaryStore
	objectStore                *MockObjectStore
	objectStoreGetter          *MockObjectStoreGetter
	bootstrapUnlocker          *MockUnlocker
	domainServices             *MockDomainServices
	controllerConfigService    *MockControllerConfigService
	cloudService               *MockCloudService
	storageService             *MockStorageService
	keyManagerService          *MockKeyManagerService
	agentPasswordService       *MockAgentPasswordService
	applicationService         *MockApplicationService
	controllerNodeService      *MockControllerNodeService
	modelConfigService         *MockModelConfigService
	modelInfoService           *MockModelInfoService
	machineService             *MockMachineService
	userService                *MockUserService
	networkService             *MockNetworkService
	bakeryConfigService        *MockBakeryConfigService
	flagService                *MockFlagService
	httpClient                 *MockHTTPClient
	httpClientGetter           *MockHTTPClientGetter

	statusHistory StatusHistory
	logger        logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dataDir = c.MkDir()

	s.controllerAgentBinaryStore = NewMockAgentBinaryStore(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.bootstrapUnlocker = NewMockUnlocker(ctrl)
	s.domainServices = NewMockDomainServices(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.cloudService = NewMockCloudService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.agentPasswordService = NewMockAgentPasswordService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.keyManagerService = NewMockKeyManagerService(ctrl)
	s.userService = NewMockUserService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.bakeryConfigService = NewMockBakeryConfigService(ctrl)
	s.flagService = NewMockFlagService(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.httpClientGetter = NewMockHTTPClientGetter(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)
	s.statusHistory = domain.NewStatusHistory(s.logger, clock.WallClock)

	c.Cleanup(func() {
		s.controllerAgentBinaryStore = nil
		s.objectStore = nil
		s.objectStoreGetter = nil
		s.bootstrapUnlocker = nil
		s.domainServices = nil
		s.controllerConfigService = nil
		s.cloudService = nil
		s.storageService = nil
		s.agentPasswordService = nil
		s.applicationService = nil
		s.controllerNodeService = nil
		s.modelConfigService = nil
		s.modelInfoService = nil
		s.machineService = nil
		s.keyManagerService = nil
		s.userService = nil
		s.networkService = nil
		s.bakeryConfigService = nil
		s.flagService = nil
		s.httpClient = nil
		s.httpClientGetter = nil

		s.logger = nil
		s.statusHistory = nil
	})

	return ctrl
}

func (s *baseSuite) expectGateUnlock() {
	s.bootstrapUnlocker.EXPECT().Unlock()
	s.domainServices.EXPECT().Flag().AnyTimes()
}
