// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
)

var (
	modelName            = "model-name"
	modelUUID            = "model-uuid"
	otherModelUUID       = "model-otheruuid"
	modelOwner           = coremodel.Qualifier("owner")
	backendVersionBinary = semversion.MustParseBinary("1.2.3-ubuntu-amd64")
	backendVersion       = backendVersionBinary.Number
)

type SourcePrecheckSuite struct {
	precheckBaseSuite

	modelUUID           coremodel.UUID
	controllerModelUUID coremodel.UUID

	controllerUpgradeService    *MockUpgradeService
	controllerStatusService     *MockStatusService
	controllerModelAgentService *MockModelAgentService
	controllerMachineService    *MockMachineService
	modelService                *MockModelService

	credentialServiceGetter     func(context.Context, coremodel.UUID) (migration.CredentialService, error)
	upgradeServiceGetter        func(context.Context, coremodel.UUID) (migration.UpgradeService, error)
	applicationServiceGetter    func(context.Context, coremodel.UUID) (migration.ApplicationService, error)
	relationServiceGetter       func(context.Context, coremodel.UUID) (migration.RelationService, error)
	statusServiceGetter         func(context.Context, coremodel.UUID) (migration.StatusService, error)
	modelAgentServiceGetter     func(context.Context, coremodel.UUID) (migration.ModelAgentService, error)
	machineServiceGetter        func(context.Context, coremodel.UUID) (migration.MachineService, error)
	modelMigrationServiceGetter func(context.Context, coremodel.UUID) (migration.ModelMigrationService, error)
}

func TestSourcePrecheckSuite(t *stdtesting.T) {
	tc.Run(t, &SourcePrecheckSuite{})
}

func (s *SourcePrecheckSuite) sourcePrecheck(
	c *tc.C,
) error {
	return migration.SourcePrecheck(
		c.Context(),
		s.modelUUID,
		s.controllerModelUUID,
		s.modelService,
		s.modelMigrationServiceGetter,
		s.credentialServiceGetter,
		s.upgradeServiceGetter,
		s.applicationServiceGetter,
		s.relationServiceGetter,
		s.statusServiceGetter,
		s.modelAgentServiceGetter,
		s.machineServiceGetter,
	)
}

func (s *SourcePrecheckSuite) expectModel() {
	m := coremodel.Model{
		Life:      corelife.Alive,
		Name:      "foo",
		Qualifier: "fred",
		UUID:      s.modelUUID,
	}
	s.modelService.EXPECT().Model(gomock.Any(), s.modelUUID).Return(m, nil)
}

func (s *SourcePrecheckSuite) expectControllerNoMachines() {
	s.controllerMachineService.EXPECT().AllMachineNames(gomock.Any()).Return(nil, nil)
}

func (s *SourcePrecheckSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.precheckBaseSuite.setupMocksWithDefaultAgentVersion(c)

	s.modelUUID = modeltesting.GenModelUUID(c)
	s.controllerModelUUID = modeltesting.GenModelUUID(c)

	s.controllerUpgradeService = NewMockUpgradeService(ctrl)
	s.controllerModelAgentService = NewMockModelAgentService(ctrl)
	s.controllerStatusService = NewMockStatusService(ctrl)
	s.controllerMachineService = NewMockMachineService(ctrl)
	s.modelService = NewMockModelService(ctrl)

	c.Cleanup(func() {
		s.controllerUpgradeService = nil
		s.controllerStatusService = nil
		s.controllerModelAgentService = nil
		s.controllerMachineService = nil
		s.modelService = nil
	})

	s.credentialServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.CredentialService, error) {
		if modelUUID == s.modelUUID {
			return s.credentialService, nil
		}
		return nil, errors.Errorf("unexpected call to applicationServiceGetter with modelUUID %q", modelUUID)
	}
	s.upgradeServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.UpgradeService, error) {
		if modelUUID == s.controllerModelUUID {
			return s.controllerUpgradeService, nil
		}
		return nil, errors.Errorf("unexpected call to upgradeServiceGetter with modelUUID %q", modelUUID)
	}
	s.applicationServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.ApplicationService, error) {
		if modelUUID == s.modelUUID {
			return s.applicationService, nil
		}
		return nil, errors.Errorf("unexpected call to applicationServiceGetter with modelUUID %q", modelUUID)
	}
	s.relationServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.RelationService, error) {
		if modelUUID == s.modelUUID {
			return s.relationService, nil
		}
		return nil, errors.Errorf("unexpected call to relationServiceGetter with modelUUID %q", modelUUID)
	}
	s.statusServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.StatusService, error) {
		if modelUUID == s.modelUUID {
			return s.statusService, nil
		} else if modelUUID == s.controllerModelUUID {
			return s.controllerStatusService, nil
		}
		return nil, errors.Errorf("unexpected call to statusServiceGetter with modelUUID %q", modelUUID)
	}
	s.modelAgentServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.ModelAgentService, error) {
		if modelUUID == s.modelUUID {
			return s.agentService, nil
		} else if modelUUID == s.controllerModelUUID {
			return s.controllerModelAgentService, nil
		}
		return nil, errors.Errorf("unexpected call to modelAgentServiceGetter with modelUUID %q", modelUUID)
	}
	s.machineServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.MachineService, error) {
		if modelUUID == s.modelUUID {
			return s.machineService, nil
		} else if modelUUID == s.controllerModelUUID {
			return s.controllerMachineService, nil
		}
		return nil, errors.Errorf("unexpected call to machineServiceGetter with modelUUID %q", modelUUID)
	}
	s.modelMigrationServiceGetter = func(_ context.Context, modelUUID coremodel.UUID) (migration.ModelMigrationService, error) {
		if modelUUID == s.modelUUID {
			return s.modelMigrationService, nil
		}
		return nil, errors.Errorf("unexpected call to modelMigrationServiceGetter with modelUUID %q", modelUUID)
	}

	return ctrl
}

func (s *SourcePrecheckSuite) TestSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectModel()
	s.expectMigrationModeNone()
	s.expectAgentVersion()
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})
	s.expectControllerNoMachines()
	s.expectNoMachines()
	s.expectNoMachines()

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.controllerStatusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerModelAgentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestDyingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := coremodel.Model{
		Life: corelife.Dying,
	}
	s.modelService.EXPECT().Model(gomock.Any(), s.modelUUID).Return(m, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "model is dying")
}

func (s *SourcePrecheckSuite) TestCharmUpgrades(c *tc.C) {
	c.Skip("(aflynn) Re-enable when upgrades is moved to dqlite.")
}

func (s *SourcePrecheckSuite) TestTargetController3Failed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return []base.Base{
			base.MustParseBaseFromString("ubuntu@24.04"),
			base.MustParseBaseFromString("ubuntu@22.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		}
	})

	s.expectModel()
	s.expectMigrationModeNone()

	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return([]coremachine.Name{"0", "1"}, nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), coremachine.Name("0")).Return(base.MustParseBaseFromString("ubuntu@22.04"), nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), coremachine.Name("1")).Return(base.MustParseBaseFromString("ubuntu@18.04"), nil)

	err := s.sourcePrecheck(c)
	c.Assert(err.Error(), tc.Equals, `
cannot migrate to controller due to issues:
"fred/foo":
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`[1:])
}

func (s *SourcePrecheckSuite) TestImportingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.modelMigrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(modelmigration.MigrationModeImporting, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "model is being imported as part of another migration")
}

func (s *SourcePrecheckSuite) TestCleanupsError(c *tc.C) {
	// TODO(modelmigration): fix cleanup check before migration
	c.Skip("fix cleanup check before migration")
	defer s.setupMocks(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})
	s.expectAllAppsAndUnitsAlive()

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	//backend.cleanupErr = errors.New("boom")
	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "checking cleanups: boom")
}

func (s *SourcePrecheckSuite) TestCleanupsNeeded(c *tc.C) {
	// TODO(modelmigration): fix cleanup check before migration
	c.Skip("fix cleanup check before migration")
	defer s.setupMocks(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})
	s.expectAllAppsAndUnitsAlive()

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	//backend.cleanupNeeded = true
	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "cleanup needed")
}

func (s *SourcePrecheckSuite) TestIsUpgradingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, errors.New("boom"))

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "controller: checking for upgrades: boom")
}

func (s *SourcePrecheckSuite) TestIsUpgrading(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(true, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "controller: upgrade in progress")
}

func (s *SourcePrecheckSuite) TestMachineRequiresReboot(c *tc.C) {
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.Skip("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	defer s.setupMocks(c).Finish()

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *SourcePrecheckSuite) TestMachineVersionsDoNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(
		[]coremachine.Name{
			coremachine.Name("1"),
		},
		nil,
	)

	err := s.sourcePrecheck(c)
	c.Check(err, tc.ErrorMatches, `there exists machines in the model that are not running the target agent version of the model \[1\]`)
}

func (s *SourcePrecheckSuite) TestMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(errors.New("boom"))

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *SourcePrecheckSuite) TestControllerMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerModelAgentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.controllerStatusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(errors.New("boom"))

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, ".*controller:.*boom")
}

func (s *SourcePrecheckSuite) TestDyingApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectDeadAppsOrUnits(errors.Errorf("application foo is dying"))
	s.expectCheckUnitStatuses(nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, ".*application foo is dying")
}

func (s *SourcePrecheckSuite) TestUnitVersionsDoNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.agentService.EXPECT().GetModelTargetAgentVersion(
		gomock.Any(),
	).Return(semversion.MustParse("4.1.1"), nil).AnyTimes()
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(
		[]coreunit.Name{
			coreunit.Name("foo/0"),
		},
		nil,
	)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	err := s.sourcePrecheck(c)
	c.Check(err, tc.ErrorMatches, `there exists units in the model that are not running the target agent version of the model \[foo/0\]`)
}

func (s *SourcePrecheckSuite) TestDeadUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectDeadAppsOrUnits(errors.Errorf("unit foo/0 is dead"))
	s.expectCheckUnitStatuses(nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, ".*unit foo/0 is dead")
}

func (s *SourcePrecheckSuite) TestUnitNotReadyForMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectCheckUnitStatuses(errors.Errorf("boom"))

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	err := s.sourcePrecheck(c)
	c.Assert(err.Error(), tc.Equals, "boom")
}

func (s *SourcePrecheckSuite) TestDyingControllerModel(c *tc.C) {
	// TODO(modelmigration): implement a way to check if the controller is dying
	// without depending on the controller model life.
	c.Skip("fix check for dying controller another way")
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectCheckUnitStatuses(nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "controller: model is dying")
}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsDoNotMatch(c *tc.C) {
	c.Skip("(tlm) Re-enable when migration is moved to dqlite.")
	defer s.setupMocks(c).Finish()

	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	//backend := newFakeBackend()
	//backend.controllerBackend = newBackendWithMismatchingTools()
	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, "controller: machine . agent binaries don't match model.+")
}

func (s *SourcePrecheckSuite) TestUnitsAllInScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectControllerNoMachines()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectAgentVersion()
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{
		eps: []relation.Endpoint{
			{
				ApplicationName: "foo",
				Relation: charm.Relation{
					Name: "db",
					Role: charm.RoleRequirer,
				},
			},
			{
				ApplicationName: "bar",
				Relation: charm.Relation{
					Name: "db",
					Role: charm.RoleProvider,
				},
			},
		},
		units:       set.NewStrings("foo/0", "bar/0", "bar/1"),
		appsToUnits: map[string][]coreunit.Name{"foo": {"foo/0"}, "bar": {"bar/0", "bar/1"}},
	})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.controllerStatusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerModelAgentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestSubordinatesNotYetInScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel()
	s.expectMigrationModeNone()
	s.expectNoMachines()
	s.expectNoMachines()
	s.expectAgentVersion()
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{
		eps: []relation.Endpoint{
			{
				ApplicationName: "foo",
				Relation: charm.Relation{
					Name: "db",
					Role: charm.RoleRequirer,
				},
			},
			{
				ApplicationName: "bar",
				Relation: charm.Relation{
					Name: "db",
					Role: charm.RoleProvider,
				},
			},
		},
		units:       set.NewStrings("foo/0", "bar/0"), // bar/1 hasn't joined yet
		appsToUnits: map[string][]coreunit.Name{"foo": {"foo/0"}, "bar": {"bar/0", "bar/1"}},
	})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	//backend := newHappyBackend()
	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, `unit bar/1 hasn't joined relation "foo:db bar:db" yet`)
}

func (s *SourcePrecheckSuite) TestCrossModelUnitsNotYetInScope(c *tc.C) {
	c.Skip("(gfouillet) Re-enable when crossmodel relation  moved to dqlite.")
	defer s.setupMocks(c).Finish()

	s.expectAgentVersion()
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})
	// todo(gfouillet) - to test CMR, mock CMR relation, once CMR implemented
	//   - application "foo" (local) and "remote-mysql" (remote)
	//   - relation "foo:db" (local) to "remote-mysql:db (remote)
	//   - unit foo/0 in scope
	//   - unit remote-mysql/0 not in scope

	//backend := newHappyBackend()
	err := s.sourcePrecheck(c)
	c.Assert(err, tc.ErrorMatches, `unit remote-mysql/0 hasn't joined relation "foo:db remote-mysql:db" yet`)
}

type ImportPrecheckSuite struct {
	precheckBaseSuite
}

func TestImportPrecheckSuite(t *stdtesting.T) {
	tc.Run(t, &ImportPrecheckSuite{})
}

func (s *ImportPrecheckSuite) TestImportPrecheckEmpty(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})
	err := migration.ImportDescriptionPrecheck(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ImportPrecheckSuite) TestCharmsWithNoManifest(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})
	// Add an app with a nil slice of bases.
	model.AddApplication(description.ApplicationArgs{
		Name: "nil-bases-app",
	}).SetCharmManifest(description.CharmManifestArgs{})

	// Add an app with an empty slice of bases.
	model.AddApplication(description.ApplicationArgs{
		Name: "empty-bases-app",
	}).SetCharmManifest(description.CharmManifestArgs{
		Bases: make([]description.CharmManifestBase, 0),
	})

	// Add an app with valid bases.
	model.AddApplication(description.ApplicationArgs{
		Name: "valid-manifest-app",
	}).SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{baseType{
			name:          "ubuntu",
			channel:       "24.04",
			architectures: []string{"amd64"},
		}},
	})

	err := migration.ImportDescriptionPrecheck(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, ".* all charms now require a manifest.yaml file, this model hosts charm\\(s\\) with no manifest.yaml file: empty-bases-app, nil-bases-app")
}

func (s *ImportPrecheckSuite) TestContainerNetworkingFan(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Config: testing.FakeConfig().Merge(testing.Attrs{"container-networking-method": "fan"}),
	})

	err := migration.ImportDescriptionPrecheck(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, ".*fan networking not supported, remove container-networking-method \"fan\" from migrating model config")
}

type baseType struct {
	name          string
	channel       string
	architectures []string
}

// Name returns the name of the base.
func (b baseType) Name() string {
	return b.name
}

// Channel returns the channel of the base.
func (b baseType) Channel() string {
	return b.channel
}

// Architectures returns the architectures of the base.
func (b baseType) Architectures() []string {
	return b.architectures
}

type TargetPrecheckSuite struct {
	precheckBaseSuite
	modelInfo coremigration.ModelInfo

	otherModelMigrationService *MockModelMigrationService
}

func TestTargetPrecheckSuite(t *stdtesting.T) {
	tc.Run(t, &TargetPrecheckSuite{})
}

func (s *TargetPrecheckSuite) SetUpTest(c *tc.C) {
	s.modelInfo = coremigration.ModelInfo{
		UUID:         modelUUID,
		Qualifier:    modelOwner,
		Name:         modelName,
		AgentVersion: backendVersion,
	}
}

func (s *TargetPrecheckSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.precheckBaseSuite.setupMocks(c)

	s.otherModelMigrationService = NewMockModelMigrationService(ctrl)

	c.Cleanup(func() {
		s.otherModelMigrationService = nil
	})

	return ctrl
}

func (s *TargetPrecheckSuite) setupMocksWithDefaultAgentVersion(c *tc.C) *gomock.Controller {
	ctrl := s.precheckBaseSuite.setupMocksWithDefaultAgentVersion(c)

	s.otherModelMigrationService = NewMockModelMigrationService(ctrl)

	c.Cleanup(func() {
		s.otherModelMigrationService = nil
	})

	return ctrl
}

func (s *TargetPrecheckSuite) runPrecheck(c *tc.C) error {
	modelMigrationServiceGetter := func(
		_ context.Context,
		m coremodel.UUID,
	) (migration.ModelMigrationService, error) {
		if m == coremodel.UUID(modelUUID) {
			return s.modelMigrationService, nil
		} else if m == coremodel.UUID(otherModelUUID) {
			return s.otherModelMigrationService, nil
		}
		return nil, errors.Errorf("unexpected call to modelMigrationServiceGetter with modelUUID %q", m)
	}

	return migration.TargetPrecheck(
		c.Context(), s.modelInfo, s.modelService, s.upgradeService,
		s.statusService, s.agentService, s.machineService,
		modelMigrationServiceGetter)
}

func (s *TargetPrecheckSuite) TestSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectNoModels()
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.expectAgentVersion()
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestModelVersionAheadOfTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.AgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(c)
	c.Assert(err.Error(), tc.Equals,
		`model has higher version than target controller (1.2.4 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMajorAhead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceVersion := backendVersion
	sourceVersion.Major++
	sourceVersion.Minor = 0
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(c)
	c.Assert(err.Error(), tc.Equals,
		`source controller has higher version than target controller (2.0.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMinorAhead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceVersion := backendVersion
	sourceVersion.Minor++
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(c)
	c.Assert(err.Error(), tc.Equals,
		`source controller has higher version than target controller (1.3.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerPatchAhead(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectNoModels()
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(c), tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerBuildAhead(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectNoModels()
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	sourceVersion := backendVersion
	sourceVersion.Build++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(c), tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerTagMismatch(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectNoModels()
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	sourceVersion := backendVersion
	sourceVersion.Tag = "alpha"
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(c), tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestDying(c *tc.C) {
	// TODO(modelmigration): implement a way to check if the controller is dying
	// without depending on the controller model life.
	c.Skip("fix check for dying controller another way")
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectNoMachines()
	//backend := newFakeBackend()
	//backend.model.life = state.Dying
	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorMatches, "model is dying")
}

func (s *TargetPrecheckSuite) TestMachineRequiresReboot(c *tc.C) {
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.Skip("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectNoMachines()
	s.expectIsUpgrade(false)

	//err := s.runPrecheck(c, newBackendWithRebootingMachine(), nil)
	//c.Assert(err, tc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *TargetPrecheckSuite) TestIsUpgradingError(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgradeError(errors.New("boom"))

	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorMatches, "checking for upgrades: boom")
}

func (s *TargetPrecheckSuite) TestIsUpgrading(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(true)

	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorMatches, "upgrade in progress")
}

func (s *TargetPrecheckSuite) TestIsMigrationActive(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	models := []coremodel.Model{
		{Name: modelName, Qualifier: modelOwner, UUID: coremodel.UUID(modelUUID), Life: corelife.Alive},
	}
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return(models, nil)
	s.modelMigrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(modelmigration.MigrationModeExporting, nil)
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	//backend := &fakeBackend{migrationActive: true}
	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorMatches, "model is being migrated out of target controller")
}

func (s *TargetPrecheckSuite) TestModelNameAlreadyInUse(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	models := []coremodel.Model{
		{Name: modelName, Qualifier: modelOwner, UUID: coremodel.UUID(otherModelUUID), Life: corelife.Alive},
	}
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return(models, nil)
	s.otherModelMigrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(modelmigration.MigrationModeNone, nil)
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	//pool := &fakePool{
	//	models: []migration.PrecheckModel{
	//		&fakeModel{
	//			uuid:      "uuid",
	//			name:      modelName,
	//			modelType: state.ModelTypeIAAS,
	//			owner:     modelOwner,
	//		},
	//	},
	//}
	//backend := newFakeBackend()
	//backend.models = pool.uuids()
	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorMatches, "model named \"model-name\" already exists")
}

func (s *TargetPrecheckSuite) TestModelNameOverlapOkForDifferentOwner(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	models := []coremodel.Model{
		{Name: modelName, Qualifier: coremodel.Qualifier("tom"), UUID: coremodel.UUID(otherModelUUID), Life: corelife.Alive},
	}
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return(models, nil)
	s.otherModelMigrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(modelmigration.MigrationModeNone, nil)
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExists(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	models := []coremodel.Model{
		{Name: modelName, Qualifier: modelOwner, UUID: coremodel.UUID(modelUUID), Life: corelife.Alive},
	}
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return(models, nil)
	s.modelMigrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(modelmigration.MigrationModeNone, nil)
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.runPrecheck(c)
	c.Assert(err.Error(), tc.Equals, "model with same UUID already exists (model-uuid)")
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExistsButImporting(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	models := []coremodel.Model{
		{Name: modelName, Qualifier: modelOwner, UUID: coremodel.UUID(modelUUID), Life: corelife.Alive},
	}
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return(models, nil)
	s.modelMigrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(modelmigration.MigrationModeImporting, nil)
	s.expectNoMachines()
	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	err := s.runPrecheck(c)
	c.Assert(err, tc.ErrorIsNil)
}
