// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
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
}

var _ = gc.Suite(&SourcePrecheckSuite{})

func sourcePrecheck(
	backend migration.PrecheckBackend,
	credentialService migration.CredentialService,
	upgradeService migration.UpgradeService,
	applicationService migration.ApplicationService,
	relationService migration.RelationService,
	statusService migration.StatusService,
	agentService migration.ModelAgentService,
) error {
	return migration.SourcePrecheck(context.Background(), backend, credentialService, upgradeService, applicationService, relationService, statusService, agentService)
}

func (s *SourcePrecheckSuite) TestSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAgentVersion()
	s.expectIsUpgrade(false)
	s.expectApplicationLife("foo", life.Alive)
	s.expectApplicationLife("bar", life.Alive)
	s.expectCheckUnitStatuses(nil)
	s.expectAgentTargetVersions(c)
	s.expectCheckRelation(nil)

	backend := newHappyBackend()
	backend.controllerBackend = newHappyBackend()
	err := migration.SourcePrecheck(context.Background(), backend, &fakeCredentialService{}, s.upgradeService,
		s.applicationService,
		s.relationService,
		s.statusService,
		s.agentService,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestDyingModel(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := newFakeBackend()
	backend.model.life = state.Dying
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "model is dying")
}

func (s *SourcePrecheckSuite) TestCharmUpgrades(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectApplicationLife("spanner", life.Alive)
	s.expectCheckUnitStatuses(nil)

	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:     "spanner",
				charmURL: "ch:spanner-3",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "spanner/0", charmURL: "ch:spanner-3"},
					&fakeUnit{name: "spanner/1", charmURL: "ch:spanner-2"},
				},
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "unit spanner/1 is upgrading")
}

func (s *SourcePrecheckSuite) TestTargetController3Failed(c *gc.C) {
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

	err := migration.SourcePrecheck(context.Background(), backend, &fakeCredentialService{}, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, `
cannot migrate to controller due to issues:
"foo/model-1":
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`[1:])
}

func (s *SourcePrecheckSuite) TestTargetController2Failed(c *gc.C) {
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
	err := migration.SourcePrecheck(context.Background(), backend, &fakeCredentialService{}, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, `
cannot migrate to controller due to issues:
"foo/model-1":
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`[1:])
}

func (s *SourcePrecheckSuite) TestImportingModel(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := newFakeBackend()
	backend.model.migrationMode = state.MigrationModeImporting
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "model is being imported as part of another migration")
}

func (s *SourcePrecheckSuite) TestCleanupsError(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	backend.cleanupErr = errors.New("boom")
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "checking cleanups: boom")
}

func (s *SourcePrecheckSuite) TestCleanupsNeeded(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	backend.cleanupNeeded = true
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "cleanup needed")
}

func (s *SourcePrecheckSuite) TestIsUpgradingError(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgradeError(errors.New("boom"))
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "controller: checking for upgrades: boom")
}

func (s *SourcePrecheckSuite) TestIsUpgrading(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(true)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "controller: upgrade in progress")
}

func (s *SourcePrecheckSuite) TestMachineRequiresReboot(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.ExpectFailure("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	s.checkRebootRequired(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestMachineVersionsDoNotMatch(c *gc.C) {
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

	err := sourcePrecheck(&backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Check(err, gc.ErrorMatches, `there exists machines in the model that are not running the target agent version of the model \[1\]`)
}

func (s *SourcePrecheckSuite) TestDyingMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := newBackendWithDyingMachine()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
}

func (s *SourcePrecheckSuite) TestNonStartedMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := newBackendWithDownMachine()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "machine 0 agent not functioning at this time (down)")
}

func (s *SourcePrecheckSuite) TestProvisioningMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	err := sourcePrecheck(newBackendWithProvisioningMachine(), &fakeCredentialService{}, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "machine 0 not running (allocating)")
}

func (s *SourcePrecheckSuite) TestDownMachineAgent(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAgentVersion()
	s.expectAgentTargetVersions(c)

	backend := newHappyBackend()
	backend.machines = []migration.PrecheckMachine{
		&fakeMachine{id: "0"},
		&fakeMachine{id: "1", status: status.Down},
	}
	err := migration.SourcePrecheck(context.Background(), backend, &fakeCredentialService{}, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "machine 1 agent not functioning at this time (down)")
}

func (s *SourcePrecheckSuite) TestDyingApplication(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectApplicationLife("foo", life.Dying)
	s.expectCheckUnitStatuses(nil)

	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "application foo is dying")
}

func (s *SourcePrecheckSuite) TestUnitVersionsDoNotMatch(c *gc.C) {
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
	s.expectCheckUnitStatuses(nil)

	backend := &fakeBackend{
		model: fakeModel{modelType: state.ModelTypeIAAS},
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:  "foo",
				units: []migration.PrecheckUnit{&fakeUnit{name: "foo/0"}},
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Check(err, gc.ErrorMatches, `there exists units in the model that are not running the target agent version of the model \[foo/0\]`)
}

func (s *SourcePrecheckSuite) TestCAASModelNoUnitVersionCheck(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.expectApplicationLife("foo", life.Alive)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := &fakeBackend{
		model: fakeModel{modelType: state.ModelTypeCAAS},
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:  "foo",
				units: []migration.PrecheckUnit{&fakeUnit{name: "foo/0", noTools: true}},
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestDeadUnit(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectApplicationLife("foo", life.Alive)
	s.expectCheckUnitStatuses(nil)

	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "foo/0", life: state.Dead},
				},
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "unit foo/0 is dead")
}

func (s *SourcePrecheckSuite) TestUnitExecuting(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.expectApplicationLife("foo", life.Alive)
	s.expectCheckUnitStatuses(nil)
	s.expectAgentTargetVersions(c)
	s.expectCheckRelation(nil)

	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "foo/0"},
				},
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestUnitNotReadyForMigration(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectCheckUnitStatuses(errors.Errorf("boom"))
	s.expectAgentTargetVersions(c)

	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "foo/0"},
				},
			},
		},
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "boom")
}

func (s *SourcePrecheckSuite) TestDyingControllerModel(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	backend.controllerBackend.model.life = state.Dying
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "controller: model is dying")
}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsDoNotMatch(c *gc.C) {
	c.Skip("(tlm) Re-enable when migration is moved to dqlite.")
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	backend.controllerBackend = newBackendWithMismatchingTools()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "controller: machine . agent binaries don't match model.+")
}

func (s *SourcePrecheckSuite) TestControllerMachineRequiresReboot(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.ExpectFailure("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	s.expectIsUpgrade(false)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := newFakeBackend()
	backend.controllerBackend = newBackendWithRebootingMachine()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is scheduled to reboot")
}

func (s *SourcePrecheckSuite) TestDyingControllerMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := &fakeBackend{
		controllerBackend: newBackendWithDyingMachine(),
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is dying")
}

func (s *SourcePrecheckSuite) TestNonStartedControllerMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := &fakeBackend{
		controllerBackend: newBackendWithDownMachine(),
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "controller: machine 0 agent not functioning at this time (down)")
}

func (s *SourcePrecheckSuite) TestProvisioningControllerMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)

	backend := &fakeBackend{
		controllerBackend: newBackendWithProvisioningMachine(),
	}
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "controller: machine 0 not running (allocating)")
}

func (s *SourcePrecheckSuite) TestUnitsAllInScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentVersion()
	s.expectIsUpgrade(false)
	s.expectApplicationLife("foo", life.Alive)
	s.expectApplicationLife("bar", life.Alive)
	s.expectCheckUnitStatuses(nil)
	s.expectAgentTargetVersions(c)
	s.expectCheckRelation(map[int]fakeRelation{
		1: {
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
			units: set.NewStrings("foo/0", "bar/0", "bar/1"),
		},
	})

	backend := newHappyBackend()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestSubordinatesNotYetInScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentVersion()
	s.expectApplicationLife("foo", life.Alive)
	s.expectApplicationLife("bar", life.Alive)
	s.expectCheckUnitStatuses(nil)
	s.expectAgentTargetVersions(c)
	s.expectCheckRelation(map[int]fakeRelation{
		1: {
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
			units: set.NewStrings("foo/0", "bar/0"), // bar/1 hasn't joined yet
		},
	})

	backend := newHappyBackend()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, `unit bar/1 hasn't joined relation "foo:db bar:db" yet`)
}

func (s *SourcePrecheckSuite) TestCrossModelUnitsNotYetInScope(c *gc.C) {
	c.Skip("(gfouillet) Re-enable when crossmodel relation  moved to dqlite.")
	defer s.setupMocks(c).Finish()

	s.expectAgentVersion()
	s.expectAgentTargetVersions(c)
	s.expectApplicationLife("foo", life.Alive)
	s.expectApplicationLife("bar", life.Alive)
	s.expectCheckUnitStatuses(nil)
	s.expectCheckRelation(nil)
	// todo(gfouillet) - to test CMR, mock CMR relation, once CMR implemented
	//   - application "foo" (local) and "remote-mysql" (remote)
	//   - relation "foo:db" (local) to "remote-mysql:db (remote)
	//   - unit foo/0 in scope
	//   - unit remote-mysql/0 not in scope

	backend := newHappyBackend()
	err := sourcePrecheck(backend, &fakeCredentialService{}, s.upgradeService, s.applicationService,
		s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, `unit remote-mysql/0 hasn't joined relation "foo:db remote-mysql:db" yet`)
}

type ImportPrecheckSuite struct {
	precheckBaseSuite
}

var _ = gc.Suite(&ImportPrecheckSuite{})

func (s *ImportPrecheckSuite) TestImportPrecheckEmpty(c *gc.C) {
	model := description.NewModel(description.ModelArgs{})
	err := migration.ImportPrecheck(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ImportPrecheckSuite) TestCharmsWithNoManifest(c *gc.C) {
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

	err := migration.ImportPrecheck(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, ".* all charms now require a manifest.yaml file, this model hosts charm\\(s\\) with no manifest.yaml file: empty-bases-app, nil-bases-app")
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

var _ = gc.Suite(&TargetPrecheckSuite{})

func (s *TargetPrecheckSuite) SetUpTest(c *gc.C) {
	s.modelInfo = coremigration.ModelInfo{
		UUID:         modelUUID,
		Owner:        modelOwner,
		Name:         modelName,
		AgentVersion: backendVersion,
	}
}

func (s *TargetPrecheckSuite) runPrecheck(
	backend migration.PrecheckBackend,
	_ migration.CredentialService,
	upgradeService migration.UpgradeService,
	applicationService migration.ApplicationService,
	relationService migration.RelationService,
	statusService migration.StatusService,
	agentService migration.ModelAgentService,
) error {
	return migration.TargetPrecheck(context.Background(), backend, nil, s.modelInfo, upgradeService, applicationService, relationService, statusService, s.agentService)
}

func (s *TargetPrecheckSuite) TestSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectIsUpgrade(false)
	s.expectAgentVersion()
	s.expectAgentTargetVersions(c)
	backend := newHappyBackend()

	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestModelVersionAheadOfTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.AgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals,
		`model has higher version than target controller (1.2.4 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMajorAhead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Major++
	sourceVersion.Minor = 0
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals,
		`source controller has higher version than target controller (2.0.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMinorAhead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Minor++
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion
	s.expectAgentVersion()

	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals,
		`source controller has higher version than target controller (1.3.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerPatchAhead(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService),
		jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerBuildAhead(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Build++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService),
		jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerTagMismatch(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Tag = "alpha"
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService),
		jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestDying(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := newFakeBackend()
	backend.model.life = state.Dying
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, gc.ErrorMatches, "model is dying")
}

func (s *TargetPrecheckSuite) TestMachineRequiresReboot(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	c.ExpectFailure("Machine reboot have been moved to dqlite, this precheck has been temporarily disabled")

	s.expectIsUpgrade(false)

	s.checkRebootRequired(c, s.runPrecheck)
}

func (s *TargetPrecheckSuite) TestIsUpgradingError(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgradeError(errors.New("boom"))

	backend := newFakeBackend()
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, gc.ErrorMatches, "checking for upgrades: boom")
}

func (s *TargetPrecheckSuite) TestIsUpgrading(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(true)

	backend := newFakeBackend()
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, gc.ErrorMatches, "upgrade in progress")
}

func (s *TargetPrecheckSuite) TestIsMigrationActiveError(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := &fakeBackend{migrationActiveErr: errors.New("boom")}
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, gc.ErrorMatches, "checking for active migration: boom")
}

func (s *TargetPrecheckSuite) TestIsMigrationActive(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	backend := &fakeBackend{migrationActive: true}
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, gc.ErrorMatches, "model is being migrated out of target controller")
}

func (s *TargetPrecheckSuite) TestDyingMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	backend := newBackendWithDyingMachine()
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
}

func (s *TargetPrecheckSuite) TestNonStartedMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	backend := newBackendWithDownMachine()
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals, "machine 0 agent not functioning at this time (down)")
}

func (s *TargetPrecheckSuite) TestProvisioningMachine(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	backend := newBackendWithProvisioningMachine()
	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals, "machine 0 not running (allocating)")
}

func (s *TargetPrecheckSuite) TestDownMachineAgent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentVersion()
	s.expectIsUpgrade(false)
	s.expectAgentTargetVersions(c)

	backend := newHappyBackend()
	backend.machines = []migration.PrecheckMachine{
		&fakeMachine{id: "0"},
		&fakeMachine{id: "1", status: status.Down},
	}
	err := migration.TargetPrecheck(context.Background(), backend, nil, s.modelInfo, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "machine 1 agent not functioning at this time (down)")
}

func (s *TargetPrecheckSuite) TestModelNameAlreadyInUse(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

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
	err := migration.TargetPrecheck(context.Background(), backend, pool, s.modelInfo, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err, gc.ErrorMatches, "model named \"model-name\" already exists")
}

func (s *TargetPrecheckSuite) TestModelNameOverlapOkForDifferentOwner(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

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
	err := migration.TargetPrecheck(context.Background(), backend, pool, s.modelInfo, s.upgradeService, s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExists(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

	pool := &fakePool{
		models: []migration.PrecheckModel{
			&fakeModel{uuid: modelUUID, modelType: state.ModelTypeIAAS},
		},
	}
	backend := newFakeBackend()
	backend.models = pool.uuids()
	err := migration.TargetPrecheck(context.Background(), backend, pool, s.modelInfo, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err.Error(), gc.Equals, "model with same UUID already exists (model-uuid)")
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExistsButImporting(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)

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
	err := migration.TargetPrecheck(context.Background(), backend, pool, s.modelInfo, s.upgradeService,
		s.applicationService, s.relationService, s.statusService, s.agentService)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestFanConfigInModelConfig(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	backend := newFakeBackend()
	s.modelInfo.ModelDescription = description.NewModel(description.ModelArgs{
		Config: testing.FakeConfig().Merge(testing.Attrs{"fan-config": "10.100.0.0/16=251.0.0.0/8 192.168.0.0/16=252.0.0.0/8"}),
	})

	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals, "fan networking not supported, remove fan-config \"10.100.0.0/16=251.0.0.0/8 192.168.0.0/16=252.0.0.0/8\" from migrating model config")
}

func (s *TargetPrecheckSuite) TestContainerNetworkingFan(c *gc.C) {
	defer s.setupMocksWithDefaultAgentVersion(c).Finish()

	s.expectIsUpgrade(false)
	backend := newFakeBackend()
	s.modelInfo.ModelDescription = description.NewModel(description.ModelArgs{
		Config: testing.FakeConfig().Merge(testing.Attrs{"container-networking-method": "fan"}),
	})

	err := s.runPrecheck(backend, nil, s.upgradeService, s.applicationService, s.relationService, s.statusService,
		s.agentService)
	c.Assert(err.Error(), gc.Equals, "fan networking not supported, remove container-networking-method \"fan\" from migrating model config")
}

type precheckRunner func(migration.PrecheckBackend, migration.CredentialService, migration.UpgradeService,
	migration.ApplicationService, migration.RelationService, migration.StatusService, migration.ModelAgentService) error

func newHappyBackend() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0"},
			&fakeMachine{id: "1"},
		},
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:  "foo",
				units: []migration.PrecheckUnit{&fakeUnit{name: "foo/0"}},
			},
			&fakeApp{
				name: "bar",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "bar/0"},
					&fakeUnit{name: "bar/1"},
				},
			},
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

func newBackendWithDyingMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", life: state.Dying},
			&fakeMachine{id: "1"},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 2},
	}
}

func newBackendWithDownMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", status: status.Down},
			&fakeMachine{id: "1"},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 2},
	}
}

func newBackendWithProvisioningMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", instanceStatus: status.Provisioning},
			&fakeMachine{id: "1"},
		},
		machineCountForSeriesUbuntu: map[string]int{"ubuntu@22.04": 2},
	}
}

type fakeCredentialService struct {
	credential     cloud.Credential
	credentialsErr error
}

func (b *fakeCredentialService) CloudCredential(_ context.Context, _ credential.Key) (cloud.Credential, error) {
	return b.credential, b.credentialsErr
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

	apps       []migration.PrecheckApplication
	allAppsErr error

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

func (b *fakeBackend) AllApplications() ([]migration.PrecheckApplication, error) {
	return b.apps, b.allAppsErr
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
		out[i] = model.UUIDOld()
	}
	return out
}

func (p *fakePool) GetModel(uuid string) (migration.PrecheckModel, func(), error) {
	for _, model := range p.models {
		if model.UUIDOld() == uuid {
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

func (m *fakeModel) TypeOld() state.ModelType {
	return m.modelType
}

func (m *fakeModel) UUIDOld() string {
	return m.uuid
}

func (m *fakeModel) NameOld() string {
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

func (m *fakeModel) CloudCredentialTagOld() (names.CloudCredentialTag, bool) {
	if names.IsValidCloudCredential(m.credential) {
		return names.NewCloudCredentialTag(m.credential), true
	}
	return names.CloudCredentialTag{}, false
}

type fakeMachine struct {
	id             string
	version        semversion.Binary
	life           state.Life
	status         status.Status
	instanceStatus status.Status
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	// rebootAction   state.RebootAction
}

func (m *fakeMachine) Id() string {
	return m.id
}

func (m *fakeMachine) Life() state.Life {
	return m.life
}

func (m *fakeMachine) Status() (status.StatusInfo, error) {
	s := m.status
	if s == "" {
		// Avoid the need to specify this everywhere.
		s = status.Started
	}
	return status.StatusInfo{Status: s}, nil
}

func (m *fakeMachine) InstanceStatus() (status.StatusInfo, error) {
	s := m.instanceStatus
	if s == "" {
		// Avoid the need to specify this everywhere.
		s = status.Running
	}
	return status.StatusInfo{Status: s}, nil
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

type fakeApp struct {
	name     string
	charmURL string
	units    []migration.PrecheckUnit
}

func (a *fakeApp) Name() string {
	return a.name
}

func (a *fakeApp) CharmURL() (*string, bool) {
	url := a.charmURL
	if url == "" {
		url = "ch:foo-1"
	}
	return &url, false
}

func (a *fakeApp) AllUnits() ([]migration.PrecheckUnit, error) {
	return a.units, nil
}

type fakeUnit struct {
	name     string
	version  semversion.Binary
	noTools  bool
	life     state.Life
	charmURL string
}

func (u *fakeUnit) Name() string {
	return u.name
}

func (u *fakeUnit) AgentTools() (*tools.Tools, error) {
	if u.noTools {
		return nil, errors.NotFoundf("tools")
	}
	// Avoid having to specify the version when it's supposed to match
	// the model config.
	v := u.version
	if v.Compare(semversion.Zero) == 0 {
		v = backendVersionBinary
	}
	return &tools.Tools{
		Version: v,
	}, nil
}

func (u *fakeUnit) Life() state.Life {
	return u.life
}

func (u *fakeUnit) ShouldBeAssigned() bool {
	return true
}

func (u *fakeUnit) CharmURL() *string {
	url := u.charmURL
	if url == "" {
		url = "ch:foo-1"
	}
	return &url
}

func (u *fakeUnit) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Idle}, nil
}

func (u *fakeUnit) IsSidecar() (bool, error) {
	return false, nil
}
