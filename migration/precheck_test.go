// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

var backendVersionBinary = version.MustParseBinary("1.2.3-trusty-amd64")
var backendVersion = backendVersionBinary.Number

type SourcePrecheckSuite struct {
	precheckBaseSuite
}

var _ = gc.Suite(&SourcePrecheckSuite{})

func sourcePrecheck(backend migration.PrecheckBackend) error {
	return migration.SourcePrecheck(backend)
}

func (*SourcePrecheckSuite) TestSuccess(c *gc.C) {
	backend := newHappyBackend()
	backend.controllerBackend = newHappyBackend()
	err := migration.SourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (*SourcePrecheckSuite) TestDyingModel(c *gc.C) {
	backend := &fakeBackend{
		modelLife: state.Dying,
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "model is dying")
}

func (*SourcePrecheckSuite) TestImportingModel(c *gc.C) {
	backend := &fakeBackend{
		migrationMode: state.MigrationModeImporting,
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "model is being imported as part of another migration")
}

func (*SourcePrecheckSuite) TestCleanupsError(c *gc.C) {
	backend := &fakeBackend{
		cleanupErr: errors.New("boom"),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "checking cleanups: boom")
}

func (*SourcePrecheckSuite) TestCleanupsNeeded(c *gc.C) {
	backend := &fakeBackend{
		cleanupNeeded: true,
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "cleanup needed")
}

func (s *SourcePrecheckSuite) TestIsUpgradingError(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: &fakeBackend{
			isUpgradingErr: errors.New("boom"),
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: checking for upgrades: boom")
}

func (s *SourcePrecheckSuite) TestIsUpgrading(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: &fakeBackend{
			isUpgrading: true,
		},
	}
	err := migration.SourcePrecheck(backend)
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
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
}

func (s *SourcePrecheckSuite) TestNonStartedMachine(c *gc.C) {
	backend := newBackendWithDownMachine()
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "machine 0 not started (down)")
}

func (s *SourcePrecheckSuite) TestProvisioningMachine(c *gc.C) {
	err := migration.SourcePrecheck(newBackendWithProvisioningMachine())
	c.Assert(err.Error(), gc.Equals, "machine 0 not running (allocating)")
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
	err := migration.SourcePrecheck(backend)
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
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "application foo is below its minimum units threshold")
}

func (s *SourcePrecheckSuite) TestUnitVersionsDontMatch(c *gc.C) {
	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name:  "foo",
				units: []migration.PrecheckUnit{&fakeUnit{name: "foo/0"}},
			},
			&fakeApp{
				name: "bar",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "bar/0"},
					&fakeUnit{name: "bar/1", version: version.MustParseBinary("1.2.4-trusty-ppc64")},
				},
			},
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "unit bar/1 tools don't match model (1.2.4 != 1.2.3)")
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
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "unit foo/0 is dead")
}

func (s *SourcePrecheckSuite) TestUnitNotIdle(c *gc.C) {
	backend := &fakeBackend{
		apps: []migration.PrecheckApplication{
			&fakeApp{
				name: "foo",
				units: []migration.PrecheckUnit{
					&fakeUnit{name: "foo/0", status: status.StatusFailed},
				},
			},
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "unit foo/0 not idle (failed)")
}

func (*SourcePrecheckSuite) TestDyingControllerModel(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: &fakeBackend{
			modelLife: state.Dying,
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: model is dying")
}

func (s *SourcePrecheckSuite) TestControllerAgentVersionError(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: &fakeBackend{
			agentVersionErr: errors.New("boom"),
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: retrieving model version: boom")

}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsDontMatch(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithMismatchingTools(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine . tools don't match model.+")
}

func (s *SourcePrecheckSuite) TestControllerMachineRequiresReboot(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithRebootingMachine(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is scheduled to reboot")
}

func (s *SourcePrecheckSuite) TestDyingControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithDyingMachine(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is dying")
}

func (s *SourcePrecheckSuite) TestNonStartedControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithDownMachine(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "controller: machine 0 not started (down)")
}

func (s *SourcePrecheckSuite) TestProvisioningControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithProvisioningMachine(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err.Error(), gc.Equals, "controller: machine 0 not running (allocating)")
}

type TargetPrecheckSuite struct {
	precheckBaseSuite
}

var _ = gc.Suite(&TargetPrecheckSuite{})

func targetPrecheck(backend migration.PrecheckBackend) error {
	return migration.TargetPrecheck(backend, backendVersion)
}

func (s *TargetPrecheckSuite) TestSuccess(c *gc.C) {
	err := migration.TargetPrecheck(newHappyBackend(), backendVersion)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TargetPrecheckSuite) TestVersionLessThanSource(c *gc.C) {
	backend := &fakeBackend{}
	err := migration.TargetPrecheck(backend, version.MustParse("1.2.4"))
	c.Assert(err.Error(), gc.Equals,
		`model has higher version than target controller (1.2.4 > 1.2.3)`)
}

func (*TargetPrecheckSuite) TestDying(c *gc.C) {
	backend := &fakeBackend{
		modelLife: state.Dying,
	}
	err := migration.TargetPrecheck(backend, backendVersion)
	c.Assert(err, gc.ErrorMatches, "model is dying")
}

func (s *TargetPrecheckSuite) TestMachineRequiresReboot(c *gc.C) {
	s.checkRebootRequired(c, targetPrecheck)
}

func (s *TargetPrecheckSuite) TestAgentVersionError(c *gc.C) {
	s.checkAgentVersionError(c, targetPrecheck)
}

func (s *TargetPrecheckSuite) TestIsUpgradingError(c *gc.C) {
	backend := &fakeBackend{
		isUpgradingErr: errors.New("boom"),
	}
	err := migration.TargetPrecheck(backend, backendVersion)
	c.Assert(err, gc.ErrorMatches, "checking for upgrades: boom")
}

func (s *TargetPrecheckSuite) TestIsUpgrading(c *gc.C) {
	backend := &fakeBackend{
		isUpgrading: true,
	}
	err := migration.TargetPrecheck(backend, backendVersion)
	c.Assert(err, gc.ErrorMatches, "upgrade in progress")
}

func (s *TargetPrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	s.checkMachineVersionsDontMatch(c, targetPrecheck)
}

func (s *TargetPrecheckSuite) TestDyingMachine(c *gc.C) {
	backend := newBackendWithDyingMachine()
	err := migration.TargetPrecheck(backend, backendVersion)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
}

func (s *TargetPrecheckSuite) TestNonStartedMachine(c *gc.C) {
	backend := newBackendWithDownMachine()
	err := migration.TargetPrecheck(backend, backendVersion)
	c.Assert(err.Error(), gc.Equals, "machine 0 not started (down)")
}

func (s *TargetPrecheckSuite) TestProvisioningMachine(c *gc.C) {
	backend := newBackendWithProvisioningMachine()
	err := migration.TargetPrecheck(backend, backendVersion)
	c.Assert(err.Error(), gc.Equals, "machine 0 not running (allocating)")
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
	c.Assert(err.Error(), gc.Equals, "machine 1 tools don't match model (1.3.1 != 1.2.3)")
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
			&fakeMachine{id: "1", version: version.MustParseBinary("1.3.1-xenial-amd64")},
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
			&fakeMachine{id: "0", status: status.StatusDown},
			&fakeMachine{id: "1"},
		},
	}
}

func newBackendWithProvisioningMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", instanceStatus: status.StatusProvisioning},
			&fakeMachine{id: "1"},
		},
	}
}

type fakeBackend struct {
	agentVersionErr error

	modelLife     state.Life
	migrationMode state.MigrationMode

	cleanupNeeded bool
	cleanupErr    error

	isUpgrading    bool
	isUpgradingErr error

	machines       []migration.PrecheckMachine
	allMachinesErr error

	apps       []migration.PrecheckApplication
	allAppsErr error

	controllerBackend *fakeBackend
}

func (b *fakeBackend) ModelLife() (state.Life, error) {
	return b.modelLife, nil
}

func (b *fakeBackend) MigrationMode() (state.MigrationMode, error) {
	return b.migrationMode, nil
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

func (b *fakeBackend) AllMachines() ([]migration.PrecheckMachine, error) {
	return b.machines, b.allMachinesErr
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
		s = status.StatusStarted
	}
	return status.StatusInfo{Status: s}, nil
}

func (m *fakeMachine) InstanceStatus() (status.StatusInfo, error) {
	s := m.instanceStatus
	if s == "" {
		// Avoid the need to specify this everywhere.
		s = status.StatusRunning
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
	units    []migration.PrecheckUnit
	minunits int
}

func (a *fakeApp) Name() string {
	return a.name
}

func (a *fakeApp) Life() state.Life {
	return a.life
}

func (a *fakeApp) AllUnits() ([]migration.PrecheckUnit, error) {
	return a.units, nil
}

func (a *fakeApp) MinUnits() int {
	return a.minunits
}

type fakeUnit struct {
	name    string
	version version.Binary
	life    state.Life
	status  status.Status
}

func (u *fakeUnit) Name() string {
	return u.name
}

func (u *fakeUnit) AgentTools() (*tools.Tools, error) {
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

func (u *fakeUnit) AgentStatus() (status.StatusInfo, error) {
	s := u.status
	if s == "" {
		// Avoid the need to specify this everywhere.
		s = status.StatusIdle
	}
	return status.StatusInfo{Status: s}, nil
}
