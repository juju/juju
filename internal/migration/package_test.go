// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	upgradevalidationmocks "github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination migration_mock_test.go github.com/juju/juju/internal/migration ControllerConfigService,UpgradeService
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ServiceFactoryGetter,ServiceFactory

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type precheckBaseSuite struct {
	testing.BaseSuite

	upgradeService *MockUpgradeService

	server        *upgradevalidationmocks.MockServer
	serverFactory *upgradevalidationmocks.MockServerFactory
}

func (s *precheckBaseSuite) checkRebootRequired(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithRebootingMachine(), &fakeCredentialService{}, s.upgradeService)
	c.Assert(err, gc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *precheckBaseSuite) checkAgentVersionError(c *gc.C, runPrecheck precheckRunner) {
	backend := &fakeBackend{
		agentVersionErr: errors.New("boom"),
	}
	err := runPrecheck(backend, &fakeCredentialService{}, s.upgradeService)
	c.Assert(err, gc.ErrorMatches, "retrieving model version: boom")
}

func (s *precheckBaseSuite) checkMachineVersionsDontMatch(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithMismatchingTools(), &fakeCredentialService{}, s.upgradeService)
	c.Assert(err.Error(), gc.Equals, "machine 1 agent binaries don't match model (1.3.1 != 1.2.3)")
}

func (s *precheckBaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.upgradeService = NewMockUpgradeService(ctrl)

	s.server = upgradevalidationmocks.NewMockServer(ctrl)
	s.serverFactory = upgradevalidationmocks.NewMockServerFactory(ctrl)
	return ctrl
}

func (s *precheckBaseSuite) expectIsUpgrade(upgrading bool) {
	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(upgrading, nil)
}

func (s *precheckBaseSuite) expectIsUpgradeError(err error) {
	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, err)
}
