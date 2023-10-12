// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"testing"

	jujutesting "github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination lock_mock_test.go github.com/juju/juju/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ControllerServiceFactory
//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination database_mock_test.go github.com/juju/juju/core/database DBGetter
//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination service_mock_test.go github.com/juju/juju/worker/upgradedatabase UpgradeService
//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination worker_mock_test.go github.com/juju/worker/v3 Worker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	lock           *MockLock
	agent          *MockAgent
	agentConfig    *MockConfig
	serviceFactory *MockControllerServiceFactory
	dbGetter       *MockDBGetter
	upgradeService *MockUpgradeService

	logger Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.serviceFactory = NewMockControllerServiceFactory(ctrl)
	s.upgradeService = NewMockUpgradeService(ctrl)
	s.dbGetter = NewMockDBGetter(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)

	return ctrl
}
