// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package upgradedatabase -destination lock_mock_test.go github.com/juju/juju/internal/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradedatabase -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradedatabase -destination servicefactory_mock_test.go github.com/juju/juju/internal/services UpgradeServices,UpgradeServicesGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradedatabase -destination database_mock_test.go github.com/juju/juju/core/database DBGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradedatabase -destination service_mock_test.go github.com/juju/juju/internal/worker/upgradedatabase UpgradeService
//go:generate go run go.uber.org/mock/mockgen -typed -package upgradedatabase -destination worker_mock_test.go github.com/juju/worker/v4 Worker

type baseSuite struct {
	lock        *MockLock
	agent       *MockAgent
	agentConfig *MockConfig

	dbGetter *MockDBGetter

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.dbGetter = NewMockDBGetter(ctrl)
	s.logger = loggertesting.WrapCheckLog(c)

	c.Cleanup(func() {
		s.lock = nil
		s.agent = nil
		s.agentConfig = nil
		s.dbGetter = nil
		s.logger = nil
	})
	return ctrl
}
