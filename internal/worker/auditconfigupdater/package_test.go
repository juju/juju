// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

//go:generate go run github.com/canonical/gomock/mockgen -package auditconfigupdater -destination servicefactory_mock_test.go github.com/juju/juju/internal/worker/auditconfigupdater ControllerConfigService

type baseSuite struct {
	testhelpers.IsolationSuite

	controllerConfigService *MockControllerConfigService
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	return ctrl
}

func (s *baseSuite) expectControllerConfig() {
	s.expectControllerConfigWithConfig(testing.FakeControllerConfig())
}

func (s *baseSuite) expectControllerConfigWithConfig(cfg controller.Config) {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)
}
