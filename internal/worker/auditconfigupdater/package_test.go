// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package auditconfigupdater -destination servicefactory_mock_test.go github.com/juju/juju/internal/worker/auditconfigupdater ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -typed -package auditconfigupdater -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config

type baseSuite struct {
	testhelpers.IsolationSuite

	agent       *MockAgent
	agentConfig *MockConfig

	controllerConfigService *MockControllerConfigService
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	return ctrl
}

func (s *baseSuite) expectControllerConfig() {
	s.expectControllerConfigWithConfig(testing.FakeControllerConfig())
}

func (s *baseSuite) expectControllerConfigWithConfig(cfg controller.Config) {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)
}
