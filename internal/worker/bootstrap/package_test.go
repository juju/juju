// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination addressfinder_mock_test.go github.com/juju/juju/environs InstanceLister
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination instance_mock_test.go github.com/juju/juju/environs/instances Instance
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination state_mock_test.go github.com/juju/juju/internal/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Unlocker
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/worker/bootstrap ControllerConfigService,FlagService,ObjectStoreGetter,SystemState,HTTPClient,CloudService,StorageService,ApplicationService,ModelConfigService,NetworkService,UserService,BakeryConfigService
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination deployer_mock_test.go github.com/juju/juju/internal/bootstrap Model

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	dataDir string

	agent                   *MockAgent
	agentConfig             *MockConfig
	state                   *MockSystemState
	stateTracker            *MockStateTracker
	objectStore             *MockObjectStore
	objectStoreGetter       *MockObjectStoreGetter
	bootstrapUnlocker       *MockUnlocker
	controllerConfigService *MockControllerConfigService
	cloudService            *MockCloudService
	storageService          *MockStorageService
	applicationService      *MockApplicationService
	modelConfigService      *MockModelConfigService
	userService             *MockUserService
	networkService          *MockNetworkService
	bakeryConfigService     *MockBakeryConfigService
	flagService             *MockFlagService
	httpClient              *MockHTTPClient
	stateModel              *MockModel

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dataDir = c.MkDir()

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.state = NewMockSystemState(ctrl)
	s.stateTracker = NewMockStateTracker(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.bootstrapUnlocker = NewMockUnlocker(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.cloudService = NewMockCloudService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.userService = NewMockUserService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.bakeryConfigService = NewMockBakeryConfigService(ctrl)
	s.flagService = NewMockFlagService(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.stateModel = NewMockModel(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) expectGateUnlock() {
	s.bootstrapUnlocker.EXPECT().Unlock()
}

func (s *baseSuite) expectAgentConfig() {
	s.agentConfig.EXPECT().DataDir().Return(s.dataDir).AnyTimes()
	s.agentConfig.EXPECT().Controller().Return(names.NewControllerTag(uuid.MustNewUUID().String())).AnyTimes()
	s.agentConfig.EXPECT().Model().Return(names.NewModelTag("test-model")).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()
}
