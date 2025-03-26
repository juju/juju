// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	stdtesting "testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testing"
	upgradevalidationmocks "github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/internal/version"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination migration_mock_test.go github.com/juju/juju/internal/migration ControllerConfigService,UpgradeService,ApplicationService,StatusService,OperationExporter,Coordinator,ModelAgentService,CharmService
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServicesGetter,DomainServices
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination description_mock_test.go github.com/juju/description/v9 Model
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type precheckBaseSuite struct {
	testing.BaseSuite

	upgradeService     *MockUpgradeService
	applicationService *MockApplicationService
	statusService      *MockStatusService
	agentService       *MockModelAgentService

	server        *upgradevalidationmocks.MockServer
	serverFactory *upgradevalidationmocks.MockServerFactory
}

func (s *precheckBaseSuite) checkRebootRequired(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithRebootingMachine(), &fakeCredentialService{}, s.upgradeService, s.applicationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *precheckBaseSuite) checkAgentVersionError(c *gc.C, runPrecheck precheckRunner, agentService migration.ModelAgentService) {
	backend := &fakeBackend{}
	err := runPrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService, s.statusService, agentService)
	c.Assert(err, gc.ErrorMatches, "retrieving model version: boom")
}

func (s *precheckBaseSuite) checkMachineVersionsDontMatch(c *gc.C, runPrecheck precheckRunner, agentService migration.ModelAgentService) {
	err := runPrecheck(newBackendWithMismatchingTools(), &fakeCredentialService{}, s.upgradeService, s.applicationService, s.statusService, agentService)
	c.Assert(err.Error(), gc.Equals, "machine 1 agent binaries don't match model (1.3.1 != 1.2.3)")
}

func (s *precheckBaseSuite) setupMocksWithDefaultAgentVersion(c *gc.C) *gomock.Controller {
	ctrl := s.setupMocks(c)
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version.MustParse("2.9.32"), nil).AnyTimes()
	return ctrl
}

func (s *precheckBaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.upgradeService = NewMockUpgradeService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)

	s.server = upgradevalidationmocks.NewMockServer(ctrl)
	s.serverFactory = upgradevalidationmocks.NewMockServerFactory(ctrl)

	return ctrl
}

func (s *precheckBaseSuite) expectApplicationLife(appName string, l life.Value) {
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), appName).Return(l, nil)
}

func (s *precheckBaseSuite) expectCheckUnitStatuses(res error) {
	s.statusService.EXPECT().CheckUnitStatusesReadyForMigration(gomock.Any()).Return(res)
}

func (s *precheckBaseSuite) expectIsUpgrade(upgrading bool) {
	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(upgrading, nil)
}

func (s *precheckBaseSuite) expectIsUpgradeError(err error) {
	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, err)
}

func (s *precheckBaseSuite) expectAgentVersion(times int) {
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version.MustParse(backendVersion.String()), nil).Times(times)
}
