// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	"github.com/juju/utils/v3"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination state_mock_test.go github.com/juju/juju/internal/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Unlocker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/worker/bootstrap ControllerConfigService,FlagService,ObjectStoreGetter,LegacyState,HTTPClient

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	agent                   *MockAgent
	agentConfig             *MockConfig
	state                   *MockLegacyState
	stateTracker            *MockStateTracker
	objectStore             *MockObjectStore
	objectStoreGetter       *MockObjectStoreGetter
	bootstrapUnlocker       *MockUnlocker
	controllerConfigService *MockControllerConfigService
	flagService             *MockFlagService
	httpClient              *MockHTTPClient

	logger Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.state = NewMockLegacyState(ctrl)
	s.stateTracker = NewMockStateTracker(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.bootstrapUnlocker = NewMockUnlocker(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.flagService = NewMockFlagService(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)

	return ctrl
}

func (s *baseSuite) expectGateUnlock() {
	s.bootstrapUnlocker.EXPECT().Unlock()
}

func (s *baseSuite) expectAgentConfig(c *gc.C) {
	s.agentConfig.EXPECT().DataDir().Return(c.MkDir()).AnyTimes()
	s.agentConfig.EXPECT().Controller().Return(names.NewControllerTag(utils.MustNewUUID().String())).AnyTimes()
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()
}
