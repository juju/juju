// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination addressfinder_mock_test.go github.com/juju/juju/environs InstanceLister
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination instance_mock_test.go github.com/juju/juju/environs/instances Instance
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination state_mock_test.go github.com/juju/juju/internal/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination storage_mock_test.go github.com/juju/juju/core/storage StorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Unlocker
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/worker/bootstrap AgentBinaryStore,ControllerConfigService,FlagService,ObjectStoreGetter,SystemState,HTTPClient,CloudService,StorageService,ApplicationService,ModelConfigService,NetworkService,UserService,BakeryConfigService,KeyManagerService,MachineService,AgentPasswordService
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination http_client_mock_test.go github.com/juju/juju/core/http HTTPClientGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServices

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	dataDir string

	agent                      *MockAgent
	agentConfig                *MockConfig
	controllerAgentBinaryStore *MockAgentBinaryStore
	state                      *MockSystemState
	stateTracker               *MockStateTracker
	objectStore                *MockObjectStore
	objectStoreGetter          *MockObjectStoreGetter
	storageRegistryGetter      *MockStorageRegistryGetter
	bootstrapUnlocker          *MockUnlocker
	domainServices             *MockDomainServices
	controllerConfigService    *MockControllerConfigService
	cloudService               *MockCloudService
	storageService             *MockStorageService
	keyManagerService          *MockKeyManagerService
	agentPasswordService       *MockAgentPasswordService
	applicationService         *MockApplicationService
	modelConfigService         *MockModelConfigService
	machineService             *MockMachineService
	userService                *MockUserService
	networkService             *MockNetworkService
	bakeryConfigService        *MockBakeryConfigService
	flagService                *MockFlagService
	httpClient                 *MockHTTPClient
	httpClientGetter           *MockHTTPClientGetter

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dataDir = c.MkDir()

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.controllerAgentBinaryStore = NewMockAgentBinaryStore(ctrl)
	s.state = NewMockSystemState(ctrl)
	s.stateTracker = NewMockStateTracker(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.storageRegistryGetter = NewMockStorageRegistryGetter(ctrl)
	s.bootstrapUnlocker = NewMockUnlocker(ctrl)
	s.domainServices = NewMockDomainServices(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.cloudService = NewMockCloudService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.agentPasswordService = NewMockAgentPasswordService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.keyManagerService = NewMockKeyManagerService(ctrl)
	s.userService = NewMockUserService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.bakeryConfigService = NewMockBakeryConfigService(ctrl)
	s.flagService = NewMockFlagService(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.httpClientGetter = NewMockHTTPClientGetter(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) expectGateUnlock() {
	s.bootstrapUnlocker.EXPECT().Unlock()
	s.domainServices.EXPECT().Flag().AnyTimes()
}

func (s *baseSuite) expectAgentConfig() {
	s.agentConfig.EXPECT().DataDir().Return(s.dataDir).AnyTimes()
	s.agentConfig.EXPECT().Controller().Return(names.NewControllerTag(uuid.MustNewUUID().String())).AnyTimes()
	s.agentConfig.EXPECT().Model().Return(names.NewModelTag("test-model")).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()
}
