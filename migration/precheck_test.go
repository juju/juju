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

func (s *SourcePrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	s.checkMachineVersionsDontMatch(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestDyingMachine(c *gc.C) {
	backend := newBackendWithDyingMachine()
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "machine 0 is dying")
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

func (s *SourcePrecheckSuite) TestDyingControllerMachine(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithDyingMachine(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "controller: machine 0 is dying")
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

type precheckRunner func(migration.PrecheckBackend) error

type precheckBaseSuite struct {
	testing.BaseSuite
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
	c.Assert(err, gc.ErrorMatches, `machine 1 tools don't match model \(1.3.1 != 1.2.3\)`)
}

func newHappyBackend() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0"},
			&fakeMachine{id: "1"},
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

func newBackendWithDyingMachine() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&fakeMachine{id: "0", life: state.Dying},
			&fakeMachine{id: "1"},
		},
	}
}

type fakeBackend struct {
	cleanupNeeded bool
	cleanupErr    error

	agentVersionErr error

	isUpgrading    bool
	isUpgradingErr error

	machines       []migration.PrecheckMachine
	allMachinesErr error

	controllerBackend *fakeBackend
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
}

func (m *fakeMachine) Id() string {
	return m.id
}

func (m *fakeMachine) Life() state.Life {
	return m.life
}

func (m *fakeMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: m.status}, nil
}

func (m *fakeMachine) InstanceStatus() (status.StatusInfo, error) {
	return status.StatusInfo{Status: m.instanceStatus}, nil
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
