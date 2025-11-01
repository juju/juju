// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeservices

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// baseSuite provides a common testing suite for the package that carries the
// dependencies needed across test suites.
type baseSuite struct {
	dbGetter              *MockWatchableDBGetter
	logger                logger.Logger
	upgradeServices       *MockUpgradeServices
	upgradeServicesGetter *MockUpgradeServicesGetter
}

// setupMocks setups mocks and basic test infra for this package.
func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbGetter = NewMockWatchableDBGetter(ctrl)
	s.logger = loggertesting.WrapCheckLog(c)
	s.upgradeServices = NewMockUpgradeServices(ctrl)
	s.upgradeServicesGetter = NewMockUpgradeServicesGetter(ctrl)

	c.Cleanup(func() {
		s.dbGetter = nil
		s.logger = nil
		s.upgradeServices = nil
		s.upgradeServicesGetter = nil
	})
	return ctrl
}
