// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination migration_mock_test.go github.com/juju/juju/internal/migration AgentBinaryStore,ControllerConfigService,UpgradeService,ApplicationService,RelationService,StatusService,OperationExporter,Coordinator,ModelAgentService,CharmService
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServicesGetter,DomainServices
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination description_mock_test.go github.com/juju/description/v9 Model
//go:generate go run go.uber.org/mock/mockgen -typed -package migration_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type precheckBaseSuite struct {
	testing.BaseSuite

	upgradeService     *MockUpgradeService
	applicationService *MockApplicationService
	relationService    *MockRelationService
	statusService      *MockStatusService
	agentService       *MockModelAgentService
}

func (s *precheckBaseSuite) checkRebootRequired(c *tc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithRebootingMachine(), &fakeCredentialService{}, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err, tc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *precheckBaseSuite) setupMocksWithDefaultAgentVersion(c *tc.C) *gomock.Controller {
	ctrl := s.setupMocks(c)
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.32"), nil).AnyTimes()
	s.expectAgentTargetVersions(c)
	return ctrl
}

func (s *precheckBaseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.upgradeService = NewMockUpgradeService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)

	return ctrl
}

func (s *precheckBaseSuite) expectApplicationLife(appName string, l life.Value) {
	s.applicationService.EXPECT().GetApplicationLife(gomock.Any(), appName).Return(l, nil)
}

func (s *precheckBaseSuite) expectCheckRelation(rels fakeRelations) {
	result := transform.MapToSlice(rels, func(k int, rel fakeRelation) []relation.RelationDetailsResult {
		return []relation.RelationDetailsResult{{
			ID:        k,
			Endpoints: rel.eps,
		}}
	})
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(result, nil)
	s.relationService.EXPECT().RelationUnitInScopeByID(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, i int, name coreunit.Name) (bool, error) {
			rel, ok := rels[i]
			if !ok {
				return false, errors.Errorf("no relation %d", i)
			}
			return rel.units.Contains(string(name)), nil
		}).AnyTimes()
}

// fakeRelation is a type that represents basic information for a relation,
// grouped by relation id.
type fakeRelations map[int]fakeRelation

type fakeRelation struct {
	eps   []relation.Endpoint
	units set.Strings
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

// expectAgentTargetVersions a hack utility function to help support
// the transition of prechecks to mocks and Dqlite. This function will take
// an established backend and setup gomock expects for machines and units to
// have their agent version information read.
func (s *precheckBaseSuite) expectAgentTargetVersions(c *tc.C) {
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).
		Return(nil, nil).AnyTimes()
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).
		Return(nil, nil).AnyTimes()
}
