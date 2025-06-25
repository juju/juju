// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/credential"
	coremachine "github.com/juju/juju/core/machine"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/state"
)

var (
	modelName            = "model-name"
	modelUUID            = "model-uuid"
	modelOwner           = names.NewUserTag("owner")
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

	credentialServiceGetter  func(context.Context, coremodel.UUID) (migration.CredentialService, error)
	upgradeServiceGetter     func(context.Context, coremodel.UUID) (migration.UpgradeService, error)
	applicationServiceGetter func(context.Context, coremodel.UUID) (migration.ApplicationService, error)
	relationServiceGetter    func(context.Context, coremodel.UUID) (migration.RelationService, error)
	statusServiceGetter      func(context.Context, coremodel.UUID) (migration.StatusService, error)
	modelAgentServiceGetter  func(context.Context, coremodel.UUID) (migration.ModelAgentService, error)
}

func TestSourcePrecheckSuite(t *stdtesting.T) {
	tc.Run(t, &SourcePrecheckSuite{})
}

func (s *SourcePrecheckSuite) sourcePrecheck(
	c *tc.C,
	backend migration.PrecheckBackend,
) error {
	return migration.SourcePrecheck(c.Context(), backend, s.modelUUID, s.controllerModelUUID, s.credentialServiceGetter,
		s.upgradeServiceGetter, s.applicationServiceGetter, s.relationServiceGetter, s.statusServiceGetter, s.modelAgentServiceGetter)
}

func (s *SourcePrecheckSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.precheckBaseSuite.setupMocksWithDefaultAgentVersion(c)

	s.modelUUID = modeltesting.GenModelUUID(c)
	s.controllerModelUUID = modeltesting.GenModelUUID(c)

	s.controllerUpgradeService = NewMockUpgradeService(ctrl)
	s.controllerModelAgentService = NewMockModelAgentService(ctrl)
	s.controllerStatusService = NewMockStatusService(ctrl)

	c.Cleanup(func() {
		s.controllerUpgradeService = nil
		s.controllerStatusService = nil
		s.controllerModelAgentService = nil
	})

	s.credentialServiceGetter = func(context.Context, coremodel.UUID) (migration.CredentialService, error) {
		return &fakeCredentialService{}, nil
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

	return ctrl
}

func (s *SourcePrecheckSuite) TestSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAgentVersion()
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.controllerStatusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerModelAgentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newHappyBackend()
	backend.controllerBackend = newHappyBackend()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestDyingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()
	backend.model.life = state.Dying
	err := s.sourcePrecheck(c, backend)
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

	backend := newFakeBackend()
	backend.machineCountForSeriesUbuntu = map[string]int{"ubuntu@22.04": 1}
	backend.machines = []migration.PrecheckMachine{
		&fakeMachine{id: "0"},
		&fakeMachine{id: "1"},
	}
	backend.model.name = "model-1"
	backend.model.owner = names.NewUserTag("foo")

	err := s.sourcePrecheck(c, backend)
	c.Assert(err.Error(), tc.Equals, `
cannot migrate to controller due to issues:
"foo/model-1":
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`[1:])
}

func (s *SourcePrecheckSuite) TestTargetController2Failed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return []base.Base{
			base.MustParseBaseFromString("ubuntu@24.04"),
			base.MustParseBaseFromString("ubuntu@22.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		}
	})

	backend := newFakeBackend()
	backend.machineCountForSeriesUbuntu = map[string]int{"ubuntu@22.04": 1}
	backend.machines = []migration.PrecheckMachine{
		&fakeMachine{id: "0"},
		&fakeMachine{id: "1"},
	}
	backend.model.name = "model-1"
	backend.model.owner = names.NewUserTag("foo")
	err := s.sourcePrecheck(c, backend)
	c.Assert(err.Error(), tc.Equals, `
cannot migrate to controller due to issues:
"foo/model-1":
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`[1:])
}

func (s *SourcePrecheckSuite) TestImportingModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()
	backend.model.migrationMode = state.MigrationModeImporting
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "model is being imported as part of another migration")
}

func (s *SourcePrecheckSuite) TestCleanupsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})
	s.expectAllAppsAndUnitsAlive()

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()
	backend.cleanupErr = errors.New("boom")
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "checking cleanups: boom")
}

func (s *SourcePrecheckSuite) TestCleanupsNeeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})
	s.expectAllAppsAndUnitsAlive()

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()
	backend.cleanupNeeded = true
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "cleanup needed")
}

func (s *SourcePrecheckSuite) TestIsUpgradingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, errors.New("boom"))

	backend := newFakeBackend()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "controller: checking for upgrades: boom")
}

func (s *SourcePrecheckSuite) TestIsUpgrading(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(true, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "controller: upgrade in progress")
}

func (s *SourcePrecheckSuite) TestMachineRequiresReboot(c *tc.C) {
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.Skip("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	defer s.setupMocks(c).Finish()

	err := s.sourcePrecheck(c, newBackendWithRebootingMachine())
	c.Assert(err, tc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *SourcePrecheckSuite) TestMachineVersionsDoNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(
		[]coremachine.Name{
			coremachine.Name("1"),
		},
		nil,
	)

	backend := fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "1"},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 2},
	}

	err := s.sourcePrecheck(c, &backend)
	c.Check(err, tc.ErrorMatches, `there exists machines in the model that are not running the target agent version of the model \[1\]`)
}

func (s *SourcePrecheckSuite) TestMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(errors.New("boom"))

	backend := newHappyBackend()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *SourcePrecheckSuite) TestControllerMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerModelAgentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.controllerStatusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(errors.New("boom"))

	backend := newHappyBackend()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, ".*controller:.*boom")
}

func (s *SourcePrecheckSuite) TestDyingApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDeadAppsOrUnits(errors.Errorf("application foo is dying"))
	s.expectCheckUnitStatuses(nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := &fakeBackend{}
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, ".*application foo is dying")
}

func (s *SourcePrecheckSuite) TestUnitVersionsDoNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	backend := &fakeBackend{
		model: fakeModel{modelType: state.ModelTypeIAAS},
	}
	err := s.sourcePrecheck(c, backend)
	c.Check(err, tc.ErrorMatches, `there exists units in the model that are not running the target agent version of the model \[foo/0\]`)
}

func (s *SourcePrecheckSuite) TestCAASModelNoUnitVersionCheck(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAllAppsAndUnitsAlive()
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.controllerModelAgentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.controllerStatusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)

	backend := &fakeBackend{
		model: fakeModel{modelType: state.ModelTypeCAAS},
	}
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestDeadUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDeadAppsOrUnits(errors.Errorf("unit foo/0 is dead"))
	s.expectCheckUnitStatuses(nil)

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := &fakeBackend{}
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, ".*unit foo/0 is dead")
}

func (s *SourcePrecheckSuite) TestUnitNotReadyForMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCheckUnitStatuses(errors.Errorf("boom"))

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	backend := &fakeBackend{}
	err := s.sourcePrecheck(c, backend)
	c.Assert(err.Error(), tc.Equals, "boom")
}

func (s *SourcePrecheckSuite) TestDyingControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectAllAppsAndUnitsAlive()
	s.expectCheckRelation(fakeRelation{})

	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.agentService.EXPECT().GetUnitsNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()
	backend.controllerBackend.model.life = state.Dying
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "controller: model is dying")
}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsDoNotMatch(c *tc.C) {
	c.Skip("(tlm) Re-enable when migration is moved to dqlite.")
	defer s.setupMocks(c).Finish()

	s.controllerUpgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(fakeRelation{})

	backend := newFakeBackend()
	backend.controllerBackend = newBackendWithMismatchingTools()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorMatches, "controller: machine . agent binaries don't match model.+")
}

func (s *SourcePrecheckSuite) TestUnitsAllInScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	backend := newHappyBackend()
	err := s.sourcePrecheck(c, backend)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestSubordinatesNotYetInScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	backend := newHappyBackend()
	err := s.sourcePrecheck(c, backend)
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

	backend := newHappyBackend()
	err := s.sourcePrecheck(c, backend)
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
}

func TestTargetPrecheckSuite(t *stdtesting.T) {
	tc.Run(t, &TargetPrecheckSuite{})
}

func (s *TargetPrecheckSuite) SetUpTest(c *tc.C) {
	s.modelInfo = coremigration.ModelInfo{
		UUID:         modelUUID,
		Qualifier:    coremodel.Qualifier(modelOwner.Id()),
		Name:         modelName,
		AgentVersion: backendVersion,
	}
}

func (s *TargetPrecheckSuite) runPrecheck(
	c *tc.C,
	backend migration.PrecheckBackend,
	pool migration.Pool,
) error {
	return migration.TargetPrecheck(c.Context(), backend, pool, s.modelInfo, s.upgradeService, s.statusService, s.agentService)
}

func (s *TargetPrecheckSuite) TestSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)
	s.expectAgentVersion()
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	backend := newHappyBackend()

	err := s.runPrecheck(c, backend, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestModelVersionAheadOfTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.AgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(c, backend, nil)
	c.Assert(err.Error(), tc.Equals,
		`model has higher version than target controller (1.2.4 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMajorAhead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Major++
	sourceVersion.Minor = 0
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(c, backend, nil)
	c.Assert(err.Error(), tc.Equals,
		`source controller has higher version than target controller (2.0.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMinorAhead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Minor++
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(c, backend, nil)
	c.Assert(err.Error(), tc.Equals,
		`source controller has higher version than target controller (1.3.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerPatchAhead(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(c, backend, nil), tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerBuildAhead(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Build++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(c, backend, nil), tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerTagMismatch(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Tag = "alpha"
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(c, backend, nil), tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestDying(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := newFakeBackend()
	backend.model.life = state.Dying
	err := s.runPrecheck(c, backend, nil)
	c.Assert(err, tc.ErrorMatches, "model is dying")
}

func (s *TargetPrecheckSuite) TestMachineRequiresReboot(c *tc.C) {
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.Skip("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	err := s.runPrecheck(c, newBackendWithRebootingMachine(), nil)
	c.Assert(err, tc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (s *TargetPrecheckSuite) TestIsUpgradingError(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgradeError(errors.New("boom"))

	backend := newFakeBackend()
	err := s.runPrecheck(c, backend, nil)
	c.Assert(err, tc.ErrorMatches, "checking for upgrades: boom")
}

func (s *TargetPrecheckSuite) TestIsUpgrading(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(true)

	backend := newFakeBackend()
	err := s.runPrecheck(c, backend, nil)
	c.Assert(err, tc.ErrorMatches, "upgrade in progress")
}

func (s *TargetPrecheckSuite) TestIsMigrationActiveError(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := &fakeBackend{migrationActiveErr: errors.New("boom")}
	err := s.runPrecheck(c, backend, nil)
	c.Assert(err, tc.ErrorMatches, "checking for active migration: boom")
}

func (s *TargetPrecheckSuite) TestIsMigrationActive(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := &fakeBackend{migrationActive: true}
	err := s.runPrecheck(c, backend, nil)
	c.Assert(err, tc.ErrorMatches, "model is being migrated out of target controller")
}

func (s *TargetPrecheckSuite) TestModelNameAlreadyInUse(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)

	pool := &fakePool{
		models: []migration.PrecheckModel{
			&fakeModel{
				uuid:      "uuid",
				name:      modelName,
				modelType: state.ModelTypeIAAS,
				owner:     modelOwner,
			},
		},
	}
	backend := newFakeBackend()
	backend.models = pool.uuids()
	err := s.runPrecheck(c, backend, pool)
	c.Assert(err, tc.ErrorMatches, "model named \"model-name\" already exists")
}

func (s *TargetPrecheckSuite) TestModelNameOverlapOkForDifferentOwner(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	pool := &fakePool{
		models: []migration.PrecheckModel{
			&fakeModel{
				name:      modelName,
				modelType: state.ModelTypeIAAS,
				owner:     names.NewUserTag("someone.else"),
			},
		},
	}
	backend := newFakeBackend()
	backend.models = pool.uuids()
	err := s.runPrecheck(c, backend, pool)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExists(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	pool := &fakePool{
		models: []migration.PrecheckModel{
			&fakeModel{uuid: modelUUID, modelType: state.ModelTypeIAAS},
		},
	}
	backend := newFakeBackend()
	backend.models = pool.uuids()
	err := s.runPrecheck(c, backend, pool)
	c.Assert(err.Error(), tc.Equals, "model with same UUID already exists (model-uuid)")
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExistsButImporting(c *tc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil)
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil)

	pool := &fakePool{
		models: []migration.PrecheckModel{
			&fakeModel{
				uuid:          modelUUID,
				modelType:     state.ModelTypeIAAS,
				migrationMode: state.MigrationModeImporting,
			},
		},
	}
	backend := newFakeBackend()
	backend.models = pool.uuids()
	err := s.runPrecheck(c, backend, pool)
	c.Assert(err, tc.ErrorIsNil)
}

func newHappyBackend() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0"},
			&fakeMachine{id: "1"},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 2},
	}
}

func newBackendWithMismatchingTools() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0"},
			&fakeMachine{id: "1", version: semversion.MustParseBinary("1.3.1-ubuntu-amd64")},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 2},
	}
}

func newBackendWithRebootingMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			// TODO(gfouillet): Restore this once machine fully migrated to dqlite
			&fakeMachine{id: "0" /*rebootAction: state.ShouldReboot*/},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 1},
	}
}

type fakeCredentialService struct {
}

func (b *fakeCredentialService) CloudCredential(_ context.Context, _ credential.Key) (cloud.Credential, error) {
	return cloud.Credential{}, nil
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		controllerBackend: &fakeBackend{},
	}
}

type fakeBackend struct {
	model  fakeModel
	models []string

	cleanupNeeded bool
	cleanupErr    error

	migrationActive    bool
	migrationActiveErr error

	machines       []migration.PrecheckMachine
	allMachinesErr error

	controllerBackend *fakeBackend

	machineCountForSeriesUbuntu map[string]int
	machineCountForSeriesErr    error

	mongoCurrentStatus    *replicaset.Status
	mongoCurrentStatusErr error
}

func (b *fakeBackend) Model() (migration.PrecheckModel, error) {
	return &b.model, nil
}

func (b *fakeBackend) AllModelUUIDs() ([]string, error) {
	return b.models, nil
}

func (b *fakeBackend) NeedsCleanup() (bool, error) {
	return b.cleanupNeeded, b.cleanupErr
}

func (b *fakeBackend) IsMigrationActive(string) (bool, error) {
	return b.migrationActive, b.migrationActiveErr
}

func (b *fakeBackend) AllMachines() ([]migration.PrecheckMachine, error) {
	return b.machines, b.allMachinesErr
}

func (b *fakeBackend) AllMachinesCount() (int, error) {
	return len(b.machines), b.allMachinesErr
}

func (b *fakeBackend) ControllerBackend() (migration.PrecheckBackend, error) {
	if b.controllerBackend == nil {
		return b, nil
	}
	return b.controllerBackend, nil
}

func (b *fakeBackend) MachineCountForBase(base ...state.Base) (map[string]int, error) {
	if b.machineCountForSeriesUbuntu == nil {
		return nil, nil
	}
	return b.machineCountForSeriesUbuntu, b.machineCountForSeriesErr
}

func (b *fakeBackend) MongoCurrentStatus() (*replicaset.Status, error) {
	if b.mongoCurrentStatus == nil {
		return &replicaset.Status{}, nil
	}
	return b.mongoCurrentStatus, b.mongoCurrentStatusErr
}

type fakePool struct {
	models []migration.PrecheckModel
}

func (p *fakePool) uuids() []string {
	out := make([]string, len(p.models))
	for i, model := range p.models {
		out[i] = model.UUID()
	}
	return out
}

func (p *fakePool) GetModel(uuid string) (migration.PrecheckModel, func(), error) {
	for _, model := range p.models {
		if model.UUID() == uuid {
			return model, func() {}, nil
		}
	}
	return nil, nil, errors.NotFoundf("model %v", uuid)
}

type fakeModel struct {
	uuid          string
	name          string
	owner         names.UserTag
	life          state.Life
	modelType     state.ModelType
	migrationMode state.MigrationMode
	credential    string
}

func (m *fakeModel) Type() state.ModelType {
	return m.modelType
}

func (m *fakeModel) UUID() string {
	return m.uuid
}

func (m *fakeModel) Name() string {
	return m.name
}

func (m *fakeModel) Owner() names.UserTag {
	return m.owner
}

func (m *fakeModel) Life() state.Life {
	return m.life
}

func (m *fakeModel) MigrationMode() (state.MigrationMode, error) {
	return m.migrationMode, nil
}

func (m *fakeModel) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	if names.IsValidCloudCredential(m.credential) {
		return names.NewCloudCredentialTag(m.credential), true
	}
	return names.CloudCredentialTag{}, false
}

type fakeMachine struct {
	id      string
	version semversion.Binary
	life    state.Life
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	// rebootAction   state.RebootAction
}

func (m *fakeMachine) Id() string {
	return m.id
}

func (m *fakeMachine) Life() state.Life {
	return m.life
}

func (m *fakeMachine) AgentTools() (*tools.Tools, error) {
	// Avoid having to specify the version when it's supposed to match
	// the model config.
	v := m.version
	if v.Compare(semversion.Zero) == 0 {
		v = backendVersionBinary
	}
	return &tools.Tools{
		Version: v,
	}, nil
}

// TODO(gfouillet): Restore this once machine fully migrated to dqlite
// func (m *fakeMachine) ShouldRebootOrShutdown() (state.RebootAction, error) {
// 	if m.rebootAction == "" {
// 		return state.ShouldDoNothing, nil
// 	}
// 	return m.rebootAction, nil
// }
