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

	"github.com/juju/juju/internal/uuid"
	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination state_mock_test.go github.com/juju/juju/internal/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Unlocker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/worker/bootstrap ControllerConfigService,FlagService,ObjectStoreGetter,SystemState,HTTPClient,CredentialService,CloudService,StorageService,ApplicationService,SpaceService
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination deployer_mock_test.go github.com/juju/juju/internal/bootstrap Model

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
	credentialService       *MockCredentialService
	storageService          *MockStorageService
	applicationService      *MockApplicationService
	spaceService            *MockSpaceService
	flagService             *MockFlagService
	httpClient              *MockHTTPClient
	stateModel              *MockModel

	logger        Logger
	loggerFactory LoggerFactory
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
	s.credentialService = NewMockCredentialService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.spaceService = NewMockSpaceService(ctrl)
	s.flagService = NewMockFlagService(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.stateModel = NewMockModel(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)
	s.loggerFactory = loggerFactory{
		logger: s.logger,
	}

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

type loggerFactory struct {
	logger Logger
}

func (f loggerFactory) Child(string) Logger {
	return f.logger
}

func (f loggerFactory) ChildWithTags(string, ...string) Logger {
	return f.logger
}

func (f loggerFactory) Namespace(string) LoggerFactory {
	return f
}
