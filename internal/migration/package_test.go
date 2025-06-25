// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination migration_mock_test.go github.com/juju/juju/internal/migration AgentBinaryStore,ControllerConfigService,UpgradeService,ApplicationService,RelationService,StatusService,OperationExporter,Coordinator,ModelAgentService,CharmService
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServicesGetter,DomainServices
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination description_mock_test.go github.com/juju/description/v10 Model
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter

type precheckBaseSuite struct {
	testing.BaseSuite

	upgradeService     *MockUpgradeService
	applicationService *MockApplicationService
	relationService    *MockRelationService
	statusService      *MockStatusService
	agentService       *MockModelAgentService
}

func (s *precheckBaseSuite) setupMocksWithDefaultAgentVersion(c *tc.C) *gomock.Controller {
	ctrl := s.setupMocks(c)
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.32"), nil).AnyTimes()
	return ctrl
}

func (s *precheckBaseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.upgradeService = NewMockUpgradeService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)

	c.Cleanup(func() {
		s.upgradeService = nil
		s.applicationService = nil
		s.relationService = nil
		s.statusService = nil
		s.agentService = nil
	})

	return ctrl
}

func (s *precheckBaseSuite) expectAllAppsAndUnitsAlive() {
	s.applicationService.EXPECT().CheckAllApplicationsAndUnitsAreAlive(gomock.Any()).Return(nil)
}

func (s *precheckBaseSuite) expectDeadAppsOrUnits(err error) {
	s.applicationService.EXPECT().CheckAllApplicationsAndUnitsAreAlive(gomock.Any()).Return(err)
}

func (s *precheckBaseSuite) expectCheckRelation(rel fakeRelation) {
	result := []relation.RelationDetailsResult{{
		ID:        1,
		Endpoints: rel.eps,
	}}

	for appName, units := range rel.appsToUnits {
		s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), appName).Return(units, nil)
	}

	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(result, nil)
	s.relationService.EXPECT().RelationUnitInScopeByID(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, i int, name coreunit.Name) (bool, error) {
			return rel.units.Contains(string(name)), nil
		}).AnyTimes()
}

type fakeRelation struct {
	eps         []relation.Endpoint
	units       set.Strings
	appsToUnits map[string][]coreunit.Name
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

func (s *precheckBaseSuite) expectAgentVersion() {
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse(backendVersion.String()), nil).AnyTimes()
}
