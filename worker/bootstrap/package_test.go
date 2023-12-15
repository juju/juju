// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination state_mock_test.go github.com/juju/juju/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination lock_mock_test.go github.com/juju/juju/worker/gate Unlocker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/worker/bootstrap ControllerConfigService,FlagService,ObjectStoreGetter,SystemState,HTTPClient

func TestPackage(t *testing.T) {
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
	flagService             *MockFlagService
	httpClient              *MockHTTPClient

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
	s.flagService = NewMockFlagService(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)
	s.loggerFactory = loggerFactory{
		logger: s.logger,
	}

	return ctrl
}

func (s *baseSuite) expectGateUnlock() {
	s.bootstrapUnlocker.EXPECT().Unlock()
}

func (s *baseSuite) expectAgentConfig(c *gc.C) {
	s.agentConfig.EXPECT().DataDir().Return(s.dataDir).AnyTimes()
	s.agentConfig.EXPECT().Controller().Return(names.NewControllerTag(utils.MustNewUUID().String())).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()
}

type loggerFactory struct {
	logger Logger
}

func (f loggerFactory) Child(string) Logger {
	return f.logger
}

func (f loggerFactory) ChildWithLabels(string, ...string) Logger {
	return f.logger
}

func (f loggerFactory) Namespace(string) LoggerFactory {
	return f
}
