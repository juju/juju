// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/replicaset/v3"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades/upgradevalidation"
	upgradevalidationmocks "github.com/juju/juju/upgrades/upgradevalidation/mocks"
)

var (
	modelName            = "model-name"
	modelUUID            = "model-uuid"
	modelOwner           = names.NewUserTag("owner")
	backendVersionBinary = version.MustParseBinary("1.2.3-ubuntu-amd64")
	backendVersion       = backendVersionBinary.Number
)

type SourcePrecheckSuite struct {
	precheckBaseSuite
}

var _ = gc.Suite(&SourcePrecheckSuite{})

func sourcePrecheck(backend migration.PrecheckBackend) error {
	return migration.SourcePrecheck(
		backend, allAlivePresence(), allAlivePresence(),
		func(names.ModelTag) (environscloudspec.CloudSpec, error) {
			return environscloudspec.CloudSpec{Type: "lxd"}, nil
		},
	)
}

func (*SourcePrecheckSuite) TestSuccess(c *gc.C) {
	backend := newHappyBackend()
	backend.controllerBackend = newHappyBackend()
	err := migration.SourcePrecheck(
		backend, allAlivePresence(), allAlivePresence(),
		func(names.ModelTag) (environscloudspec.CloudSpec, error) {
			return environscloudspec.CloudSpec{Type: "lxd"}, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (*SourcePrecheckSuite) TestDyingModel(c *gc.C) {
	backend := newFakeBackend()
	backend.model.life = state.Dying
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "model is dying")
}

func (*SourcePrecheckSuite) TestCharmUpgrades(c *gc.C) {
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
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "unit spanner/1 is upgrading")
}

func (s *SourcePrecheckSuite) TestTargetController3Failed(c *gc.C) {
	ctrl := gomock.NewController(c)
	server := upgradevalidationmocks.NewMockServer(ctrl)
	serverFactory := upgradevalidationmocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}

	backend := newFakeBackend()
	hasUpgradeSeriesLocks := true
	backend.hasUpgradeSeriesLocks = &hasUpgradeSeriesLocks
	backend.machineCountForSeriesWin = map[string]int{"win10": 1, "win7": 2}
	backend.machineCountForSeriesUbuntu = map[string]int{"xenial": 1, "vivid": 2, "trusty": 3}
	agentVersion := version.MustParse("2.9.35")
	backend.model.agentVersion = &agentVersion
	backend.model.name = "model-1"
	backend.model.owner = names.NewUserTag("foo")

	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("4.0")

	err := migration.SourcePrecheck(
		backend, allAlivePresence(), allAlivePresence(),
		func(names.ModelTag) (environscloudspec.CloudSpec, error) {
			return cloudSpec.CloudSpec, nil
		},
	)
	c.Assert(err.Error(), gc.Equals, `
cannot migrate to controller due to issues:
"foo/model-1":
- unexpected upgrade series lock found
- the model hosts deprecated windows machine(s): win10(1) win7(2)
- the model hosts deprecated ubuntu machine(s): trusty(3) vivid(2) xenial(1)
- LXD version has to be at least "5.0.0", but current version is only "4.0.0"`[1:])
}

func (*SourcePrecheckSuite) TestTargetController2Failed(c *gc.C) {
	backend := newFakeBackend()
	hasUpgradeSeriesLocks := true
	backend.hasUpgradeSeriesLocks = &hasUpgradeSeriesLocks
	backend.machineCountForSeriesWin = map[string]int{"win10": 1, "win7": 2}
	backend.machineCountForSeriesUbuntu = map[string]int{"xenial": 1, "vivid": 2, "trusty": 3}
	agentVersion := version.MustParse("2.9.31")
	backend.model.agentVersion = &agentVersion
	backend.model.name = "model-1"
	backend.model.owner = names.NewUserTag("foo")
	err := migration.SourcePrecheck(
		backend, allAlivePresence(), allAlivePresence(),
		func(names.ModelTag) (environscloudspec.CloudSpec, error) {
			return environscloudspec.CloudSpec{Type: "lxd"}, nil
		},
	)
	c.Assert(err.Error(), gc.Equals, `
cannot migrate to controller due to issues:
"foo/model-1":
- unexpected upgrade series lock found
- the model hosts deprecated windows machine(s): win10(1) win7(2)
- the model hosts deprecated ubuntu machine(s): trusty(3) vivid(2) xenial(1)`[1:])
}

func (*SourcePrecheckSuite) TestImportingModel(c *gc.C) {
	backend := newFakeBackend()
	backend.model.migrationMode = state.MigrationModeImporting
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "model is being imported as part of another migration")
}

func (*SourcePrecheckSuite) TestCleanupsError(c *gc.C) {
	backend := newFakeBackend()
	backend.cleanupErr = errors.New("boom")
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "checking cleanups: boom")
}

func (*SourcePrecheckSuite) TestCleanupsNeeded(c *gc.C) {
	backend := newFakeBackend()
	backend.cleanupNeeded = true
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "cleanup needed")
}

func (s *SourcePrecheckSuite) TestIsUpgradingError(c *gc.C) {
	backend := newFakeBackend()
	backend.controllerBackend.isUpgradingErr = errors.New("boom")
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: checking for upgrades: boom")
}

func (s *SourcePrecheckSuite) TestIsUpgrading(c *gc.C) {
	backend := newFakeBackend()
	backend.controllerBackend.isUpgrading = true
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: upgrade in progress")
}

func (s *SourcePrecheckSuite) TestAgentVersionError(c *gc.C) {
	s.checkAgentVersionError(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestMachineRequiresReboot(c *gc.C) {
	s.checkRebootRequired(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	s.checkMachineVersionsDontMatch(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestDyingMachine(c *gc.C) {
	backend := newBackendWithDyingMachine()
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
}

func (s *SourcePrecheckSuite) TestNonStartedMachine(c *gc.C) {
	backend := newBackendWithDownMachine()
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "machine 0 agent not functioning at this time (down)")
}

func (s *SourcePrecheckSuite) TestProvisioningMachine(c *gc.C) {
	err := sourcePrecheck(newBackendWithProvisioningMachine())
	c.Assert(err.Error(), gc.Equals, "machine 0 not running (allocating)")
}

func (s *SourcePrecheckSuite) TestDownMachineAgent(c *gc.C) {
	backend := newHappyBackend()
	modelPresence := downAgentPresence("machine-1")
	controllerPresence := allAlivePresence()
	err := migration.SourcePrecheck(
		backend, modelPresence, controllerPresence,
		func(names.ModelTag) (environscloudspec.CloudSpec, error) {
			return environscloudspec.CloudSpec{Type: "foo"}, nil
		},
	)
	c.Assert(err.Error(), gc.Equals, "machine 1 agent not functioning at this time (down)")
}

func (s *SourcePrecheckSuite) TestDyingApplication(c *gc.C) {
	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				life: state.Dying,
			},
		},
	}
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "application foo is dying")
}

func (s *SourcePrecheckSuite) TestWithPendingMinUnits(c *gc.C) {
	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:     "foo",
				minunits: 2,
				units:    []migration.PrecheckUnit{&fakeUnit{name: "foo/0"}},
			},
		},
	}
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "application foo is below its minimum units threshold")
}

func (s *SourcePrecheckSuite) TestUnitVersionsDontMatch(c *gc.C) {
	backend := &fakeBackend{
		model: fakeModel{modelType: state.ModelTypeIAAS},
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:  "foo",
				units: []migration.PrecheckUnit{&fakeUnit{name: "foo/0"}},
			},
			&fakeApp{
				name: "bar",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "bar/0"},
					&fakeUnit{name: "bar/1", version: version.MustParseBinary("1.2.4-ubuntu-ppc64")},
				},
			},
		},
	}
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "unit bar/1 agent binaries don't match model (1.2.4 != 1.2.3)")
}

func (s *SourcePrecheckSuite) TestCAASModelNoUnitVersionCheck(c *gc.C) {
	backend := &fakeBackend{
		model: fakeModel{modelType: state.ModelTypeCAAS},
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:  "foo",
				units: []migration.PrecheckUnit{&fakeUnit{name: "foo/0", noTools: true}},
			},
		},
	}
	err := sourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestDeadUnit(c *gc.C) {
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
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "unit foo/0 is dead")
}

func (s *SourcePrecheckSuite) TestUnitExecuting(c *gc.C) {
	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "foo/0", agentStatus: status.Executing},
				},
			},
		},
	}
	err := sourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestUnitNotIdle(c *gc.C) {
	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "foo/0", agentStatus: status.Failed},
				},
			},
		},
	}
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "unit foo/0 not idle or executing (failed)")
}

func (s *SourcePrecheckSuite) TestUnitLost(c *gc.C) {
	backend := newHappyBackend()
	modelPresence := downAgentPresence("unit-foo-0")
	controllerPresence := allAlivePresence()
	err := migration.SourcePrecheck(
		backend, modelPresence, controllerPresence,
		func(names.ModelTag) (environscloudspec.CloudSpec, error) {
			return environscloudspec.CloudSpec{Type: "foo"}, nil
		},
	)
	c.Assert(err.Error(), gc.Equals, "unit foo/0 not idle or executing (lost)")
}

func (*SourcePrecheckSuite) TestDyingControllerModel(c *gc.C) {
	backend := newFakeBackend()
	backend.controllerBackend.model.life = state.Dying
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: model is dying")
}

func (s *SourcePrecheckSuite) TestControllerAgentVersionError(c *gc.C) {
	backend := newFakeBackend()
	backend.controllerBackend.agentVersionErr = errors.New("boom")
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: retrieving model version: boom")

}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsDontMatch(c *gc.C) {
	backend := newFakeBackend()
	backend.controllerBackend = newBackendWithMismatchingTools()
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine . agent binaries don't match model.+")
}

func (s *SourcePrecheckSuite) TestControllerMachineRequiresReboot(c *gc.C) {
	backend := newFakeBackend()
	backend.controllerBackend = newBackendWithRebootingMachine()
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is scheduled to reboot")
}

func (s *SourcePrecheckSuite) TestDyingControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithDyingMachine(),
	}
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is dying")
}

func (s *SourcePrecheckSuite) TestNonStartedControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithDownMachine(),
	}
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "controller: machine 0 agent not functioning at this time (down)")
}

func (s *SourcePrecheckSuite) TestProvisioningControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithProvisioningMachine(),
	}
	err := sourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "controller: machine 0 not running (allocating)")
}

func (s *SourcePrecheckSuite) TestUnitsAllInScope(c *gc.C) {
	backend := newHappyBackend()
	backend.relations = []migration.PrecheckRelation{&fakeRelation{
		endpoints: []state.Endpoint{
			{ApplicationName: "foo"},
			{ApplicationName: "bar"},
		},
		relUnits: map[string]*fakeRelationUnit{
			"foo/0": {valid: true, inScope: true},
			"bar/0": {valid: true, inScope: true},
			"bar/1": {valid: true, inScope: true},
		},
	}}
	err := sourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestSubordinatesNotYetInScope(c *gc.C) {
	backend := newHappyBackend()
	backend.relations = []migration.PrecheckRelation{&fakeRelation{
		key: "foo:db bar:db",
		endpoints: []state.Endpoint{
			{ApplicationName: "foo"},
			{ApplicationName: "bar"},
		},
		relUnits: map[string]*fakeRelationUnit{
			"foo/0": {unitName: "foo/0", valid: true, inScope: true},
			"bar/0": {unitName: "bar/0", valid: true, inScope: true},
			"bar/1": {unitName: "bar/1", valid: true, inScope: false},
		},
	}}
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, `unit bar/1 hasn't joined relation "foo:db bar:db" yet`)
}

func (s *SourcePrecheckSuite) TestSubordinatesInvalidUnitsNotYetInScope(c *gc.C) {
	backend := newHappyBackend()
	backend.relations = []migration.PrecheckRelation{&fakeRelation{
		key: "foo:db bar:db",
		endpoints: []state.Endpoint{
			{ApplicationName: "foo"},
			{ApplicationName: "bar"},
		},
		relUnits: map[string]*fakeRelationUnit{
			"foo/0": {valid: true, inScope: true},
			"bar/0": {valid: true, inScope: true},
			"bar/1": {valid: false, inScope: false},
		},
	}}
	err := sourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestCrossModelUnitsNotYetInScope(c *gc.C) {
	backend := newHappyBackend()
	backend.relations = []migration.PrecheckRelation{&fakeRelation{
		key: "foo:db remote-mysql:db",
		endpoints: []state.Endpoint{
			{ApplicationName: "foo"},
			{ApplicationName: "remote-mysql"},
		},
		relUnits: map[string]*fakeRelationUnit{
			"foo/0": {unitName: "foo/0", valid: true, inScope: true},
		},
		remoteAppName: "remote-mysql",
		remoteRelUnits: map[string][]*fakeRelationUnit{
			"remote-mysql": {{unitName: "remote-mysql/0", valid: true, inScope: false}},
		},
	}}
	err := sourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, `unit remote-mysql/0 hasn't joined relation "foo:db remote-mysql:db" yet`)
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

func (s *TargetPrecheckSuite) runPrecheck(backend migration.PrecheckBackend) error {
	return migration.TargetPrecheck(backend, nil, s.modelInfo, allAlivePresence())
}

func (s *TargetPrecheckSuite) TestSuccess(c *gc.C) {
	err := s.runPrecheck(newHappyBackend())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestModelVersionAheadOfTarget(c *gc.C) {
	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.AgentVersion = sourceVersion

	err := s.runPrecheck(backend)
	c.Assert(err.Error(), gc.Equals,
		`model has higher version than target controller (1.2.4 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestModelMinimumVersion(c *gc.C) {
	backend := newFakeBackend()

	origBackendBinary := backendVersionBinary
	origBackend := backendVersion
	backendVersionBinary = version.MustParseBinary("3.0.0-ubuntu-amd64")
	backendVersion = backendVersionBinary.Number
	defer func() {
		backendVersionBinary = origBackendBinary
		backendVersion = origBackend
	}()

	s.modelInfo.AgentVersion = version.MustParse("2.8.0")
	err := s.runPrecheck(backend)
	c.Assert(err.Error(), gc.Equals,
		`model must be upgraded to at least version 2.9.43 before being migrated to a controller with version 3.0.0`)

	s.modelInfo.AgentVersion = version.MustParse("2.9.43")
	err = s.runPrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerMajorAhead(c *gc.C) {
	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Major++
	sourceVersion.Minor = 0
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion

	err := s.runPrecheck(backend)
	c.Assert(err.Error(), gc.Equals,
		`source controller has higher version than target controller (2.0.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerMinorAhead(c *gc.C) {
	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Minor++
	sourceVersion.Patch = 0
	s.modelInfo.ControllerAgentVersion = sourceVersion

	err := s.runPrecheck(backend)
	c.Assert(err.Error(), gc.Equals,
		`source controller has higher version than target controller (1.3.0 > 1.2.3)`)
}

func (s *TargetPrecheckSuite) TestSourceControllerPatchAhead(c *gc.C) {
	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Patch++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(backend), jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerBuildAhead(c *gc.C) {
	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Build++
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(backend), jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestSourceControllerTagMismatch(c *gc.C) {
	backend := newFakeBackend()

	sourceVersion := backendVersion
	sourceVersion.Tag = "alpha"
	s.modelInfo.ControllerAgentVersion = sourceVersion

	c.Assert(s.runPrecheck(backend), jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestDying(c *gc.C) {
	backend := newFakeBackend()
	backend.model.life = state.Dying
	err := s.runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "model is dying")
}

func (s *TargetPrecheckSuite) TestMachineRequiresReboot(c *gc.C) {
	s.checkRebootRequired(c, s.runPrecheck)
}

func (s *TargetPrecheckSuite) TestAgentVersionError(c *gc.C) {
	s.checkAgentVersionError(c, s.runPrecheck)
}

func (s *TargetPrecheckSuite) TestIsUpgradingError(c *gc.C) {
	backend := &fakeBackend{
		isUpgradingErr: errors.New("boom"),
	}
	err := s.runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "checking for upgrades: boom")
}

func (s *TargetPrecheckSuite) TestIsUpgrading(c *gc.C) {
	backend := &fakeBackend{
		isUpgrading: true,
	}
	err := s.runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "upgrade in progress")
}

func (s *TargetPrecheckSuite) TestIsMigrationActiveError(c *gc.C) {
	backend := &fakeBackend{migrationActiveErr: errors.New("boom")}
	err := s.runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "checking for active migration: boom")
}

func (s *TargetPrecheckSuite) TestIsMigrationActive(c *gc.C) {
	backend := &fakeBackend{migrationActive: true}
	err := s.runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "model is being migrated out of target controller")
}

func (s *TargetPrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	s.checkMachineVersionsDontMatch(c, s.runPrecheck)
}

func (s *TargetPrecheckSuite) TestDyingMachine(c *gc.C) {
	backend := newBackendWithDyingMachine()
	err := s.runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
}

func (s *TargetPrecheckSuite) TestNonStartedMachine(c *gc.C) {
	backend := newBackendWithDownMachine()
	err := s.runPrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "machine 0 agent not functioning at this time (down)")
}

func (s *TargetPrecheckSuite) TestProvisioningMachine(c *gc.C) {
	backend := newBackendWithProvisioningMachine()
	err := s.runPrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "machine 0 not running (allocating)")
}

func (s *TargetPrecheckSuite) TestDownMachineAgent(c *gc.C) {
	backend := newHappyBackend()
	modelPresence := downAgentPresence("machine-1")
	err := migration.TargetPrecheck(backend, nil, s.modelInfo, modelPresence)
	c.Assert(err.Error(), gc.Equals, "machine 1 agent not functioning at this time (down)")
}

func (s *TargetPrecheckSuite) TestModelNameAlreadyInUse(c *gc.C) {
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
	err := migration.TargetPrecheck(backend, pool, s.modelInfo, allAlivePresence())
	c.Assert(err, gc.ErrorMatches, "model named \"model-name\" already exists")
}

func (s *TargetPrecheckSuite) TestModelNameOverlapOkForDifferentOwner(c *gc.C) {
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
	err := migration.TargetPrecheck(backend, pool, s.modelInfo, allAlivePresence())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExists(c *gc.C) {
	pool := &fakePool{
		models: []migration.PrecheckModel{
			&fakeModel{uuid: modelUUID, modelType: state.ModelTypeIAAS},
		},
	}
	backend := newFakeBackend()
	backend.models = pool.uuids()
	err := migration.TargetPrecheck(backend, pool, s.modelInfo, allAlivePresence())
	c.Assert(err.Error(), gc.Equals, "model with same UUID already exists (model-uuid)")
}

func (s *TargetPrecheckSuite) TestUUIDAlreadyExistsButImporting(c *gc.C) {
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
	err := migration.TargetPrecheck(backend, pool, s.modelInfo, allAlivePresence())
	c.Assert(err, jc.ErrorIsNil)
}

type precheckRunner func(migration.PrecheckBackend) error

type precheckBaseSuite struct {
	testing.BaseSuite
}

func (*precheckBaseSuite) checkRebootRequired(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithRebootingMachine())
	c.Assert(err, gc.ErrorMatches, "machine 0 is scheduled to reboot")
}

func (*precheckBaseSuite) checkAgentVersionError(c *gc.C, runPrecheck precheckRunner) {
	backend := &fakeBackend{
		agentVersionErr: errors.New("boom"),
	}
	err := runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "retrieving model version: boom")
}

func (*precheckBaseSuite) checkMachineVersionsDontMatch(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithMismatchingTools())
	c.Assert(err.Error(), gc.Equals, "machine 1 agent binaries don't match model (1.3.1 != 1.2.3)")
}

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
	}
}

func newBackendWithMismatchingTools() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0"},
			&fakeMachine{id: "1", version: version.MustParseBinary("1.3.1-ubuntu-amd64")},
		},
	}
}

func newBackendWithRebootingMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", rebootAction: state.ShouldReboot},
		},
	}
}

func newBackendWithDyingMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", life: state.Dying},
			&fakeMachine{id: "1"},
		},
	}
}

func newBackendWithDownMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", status: status.Down},
			&fakeMachine{id: "1"},
		},
	}
}

func newBackendWithProvisioningMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", instanceStatus: status.Provisioning},
			&fakeMachine{id: "1"},
		},
	}
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		controllerBackend: &fakeBackend{},
	}
}

type fakeBackend struct {
	agentVersionErr error

	model  fakeModel
	models []string

	cleanupNeeded bool
	cleanupErr    error

	isUpgrading    bool
	isUpgradingErr error

	migrationActive    bool
	migrationActiveErr error

	machines       []migration.PrecheckMachine
	allMachinesErr error

	apps       []migration.PrecheckApplication
	allAppsErr error

	relations  []migration.PrecheckRelation
	allRelsErr error

	credentials    state.Credential
	credentialsErr error

	controllerBackend *fakeBackend

	hasUpgradeSeriesLocks    *bool
	hasUpgradeSeriesLocksErr error

	machineCountForSeriesWin    map[string]int
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

func (b *fakeBackend) AgentVersion() (version.Number, error) {
	return backendVersion, b.agentVersionErr
}

func (b *fakeBackend) IsUpgrading() (bool, error) {
	return b.isUpgrading, b.isUpgradingErr
}

func (b *fakeBackend) IsMigrationActive(string) (bool, error) {
	return b.migrationActive, b.migrationActiveErr
}

func (b *fakeBackend) CloudCredential(_ names.CloudCredentialTag) (state.Credential, error) {
	return b.credentials, b.credentialsErr
}

func (b *fakeBackend) AllMachines() ([]migration.PrecheckMachine, error) {
	return b.machines, b.allMachinesErr
}

func (b *fakeBackend) AllApplications() ([]migration.PrecheckApplication, error) {
	return b.apps, b.allAppsErr
}

func (b *fakeBackend) AllRelations() ([]migration.PrecheckRelation, error) {
	return b.relations, b.allRelsErr
}

func (b *fakeBackend) ControllerBackend() (migration.PrecheckBackend, error) {
	if b.controllerBackend == nil {
		return b, nil
	}
	return b.controllerBackend, nil
}

func (b *fakeBackend) HasUpgradeSeriesLocks() (bool, error) {
	if b.hasUpgradeSeriesLocks == nil {
		return false, nil
	}
	return *b.hasUpgradeSeriesLocks, b.hasUpgradeSeriesLocksErr
}

func (b *fakeBackend) MachineCountForBase(base ...state.Base) (map[string]int, error) {
	if strings.HasPrefix(base[0].Channel, "win") {
		if b.machineCountForSeriesWin == nil {
			return nil, nil
		}
		return b.machineCountForSeriesWin, b.machineCountForSeriesErr
	}
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

func (b *fakeBackend) AllCharmURLs() ([]*string, error) {
	return nil, errors.NotFoundf("charms")
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

	agentVersion *version.Number
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

func (m *fakeModel) MigrationMode() state.MigrationMode {
	return m.migrationMode
}

func (m *fakeModel) AgentVersion() (version.Number, error) {
	if m.agentVersion == nil {
		return version.MustParse("2.9.32"), nil
	}
	return *m.agentVersion, nil
}

func (m *fakeModel) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	if names.IsValidCloudCredential(m.credential) {
		return names.NewCloudCredentialTag(m.credential), true
	}
	return names.CloudCredentialTag{}, false
}

type fakeMachine struct {
	id             string
	version        version.Binary
	life           state.Life
	status         status.Status
	instanceStatus status.Status
	rebootAction   state.RebootAction
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
	if v.Compare(version.Zero) == 0 {
		v = backendVersionBinary
	}
	return &tools.Tools{
		Version: v,
	}, nil
}

func (m *fakeMachine) ShouldRebootOrShutdown() (state.RebootAction, error) {
	if m.rebootAction == "" {
		return state.ShouldDoNothing, nil
	}
	return m.rebootAction, nil
}

type fakeApp struct {
	name     string
	life     state.Life
	charmURL string
	units    []migration.PrecheckUnit
	minunits int
}

func (a *fakeApp) Name() string {
	return a.name
}

func (a *fakeApp) Life() state.Life {
	return a.life
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

func (a *fakeApp) MinUnits() int {
	return a.minunits
}

type fakeUnit struct {
	name        string
	version     version.Binary
	noTools     bool
	life        state.Life
	charmURL    string
	agentStatus status.Status
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
	if v.Compare(version.Zero) == 0 {
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

func (u *fakeUnit) AgentStatus() (status.StatusInfo, error) {
	s := u.agentStatus
	if s == "" {
		// Avoid the need to specify this everywhere.
		s = status.Idle
	}
	return status.StatusInfo{Status: s}, nil
}

func (u *fakeUnit) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Idle}, nil
}

func (u *fakeUnit) IsSidecar() (bool, error) {
	return false, nil
}

type fakeRelation struct {
	key            string
	endpoints      []state.Endpoint
	relUnits       map[string]*fakeRelationUnit
	remoteAppName  string
	remoteRelUnits map[string][]*fakeRelationUnit
	unitErr        error
}

func (r *fakeRelation) String() string {
	return r.key
}

func (r *fakeRelation) Endpoints() []state.Endpoint {
	return r.endpoints
}

func (r *fakeRelation) AllRemoteUnits(appName string) ([]migration.PrecheckRelationUnit, error) {
	out := make([]migration.PrecheckRelationUnit, len(r.remoteRelUnits[appName]))
	for i, ru := range r.remoteRelUnits[appName] {
		out[i] = ru
	}
	return out, nil
}

func (r *fakeRelation) RemoteApplication() (string, bool, error) {
	return r.remoteAppName, r.remoteAppName != "", nil
}

func (r *fakeRelation) Unit(u migration.PrecheckUnit) (migration.PrecheckRelationUnit, error) {
	return r.relUnits[u.Name()], r.unitErr
}

type fakeRelationUnit struct {
	unitName           string
	valid, inScope     bool
	validErr, scopeErr error
}

func (ru *fakeRelationUnit) UnitName() string {
	return ru.unitName
}

func (ru *fakeRelationUnit) Valid() (bool, error) {
	return ru.valid, ru.validErr
}

func (ru *fakeRelationUnit) InScope() (bool, error) {
	return ru.inScope, ru.scopeErr
}

func allAlivePresence() migration.ModelPresence {
	return &fakePresence{}
}

func downAgentPresence(agent ...string) migration.ModelPresence {
	m := make(map[string]presence.Status)
	for _, a := range agent {
		m[a] = presence.Missing
	}
	return &fakePresence{
		agentStatus: m,
	}
}

type fakePresence struct {
	agentStatus map[string]presence.Status
}

func (f *fakePresence) AgentStatus(agent string) (presence.Status, error) {
	if value, found := f.agentStatus[agent]; found {
		return value, nil
	}
	return presence.Alive, nil
}
