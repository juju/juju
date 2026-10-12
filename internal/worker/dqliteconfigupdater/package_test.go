// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqliteconfigupdater

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package dqliteconfigupdater -destination service_mock_test.go github.com/juju/juju/internal/worker/dqliteconfigupdater ControllerConfigService

type baseSuite struct {
	testhelpers.IsolationSuite

	controllerConfigService *MockControllerConfigService
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	return ctrl
}
