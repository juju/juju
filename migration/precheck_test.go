// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/migration"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

var backendVersion = version.MustParse("1.2.3")

type precheckBaseSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&precheckBaseSuite{})

type SourcePrecheckSuite struct {
	precheckBaseSuite
}

var _ = gc.Suite(&SourcePrecheckSuite{})

func sourcePrecheck(backend migration.PrecheckBackend) error {
	return migration.SourcePrecheck(backend)
}

func (*SourcePrecheckSuite) TestCleanups(c *gc.C) {
	backend := &fakeBackend{}
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

func (s *SourcePrecheckSuite) TestAgentVersionError(c *gc.C) {
	s.checkAgentVersionError(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestMachineVersionsMatch(c *gc.C) {
	s.checkMachineVersionsMatch(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	s.checkMachineVersionsDontMatch(c, sourcePrecheck)
}

func (s *SourcePrecheckSuite) TestControllerAgentVersionError(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: &fakeBackend{
			agentVersionErr: errors.New("boom"),
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "source controller: retrieving model version: boom")

}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsMatch(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithMatchingTools(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SourcePrecheckSuite) TestControllerMachineVersionsDontMatch(c *gc.C) {
	backend := &fakeBackend{
		controllerBackend: newBackendWithMismatchingTools(),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "source controller: machine . tools don't match model.+")
}

type TargetPrecheckSuite struct {
	precheckBaseSuite
}

var _ = gc.Suite(&TargetPrecheckSuite{})

func targetPrecheck(backend migration.PrecheckBackend) error {
	return migration.TargetPrecheck(backend, backendVersion)
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

func (s *TargetPrecheckSuite) TestMachineVersionsMatch(c *gc.C) {
	s.checkMachineVersionsMatch(c, targetPrecheck)
}

func (s *TargetPrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	s.checkMachineVersionsDontMatch(c, targetPrecheck)
}

type precheckRunner func(migration.PrecheckBackend) error

func (*precheckBaseSuite) checkAgentVersionError(c *gc.C, runPrecheck precheckRunner) {
	backend := &fakeBackend{
		agentVersionErr: errors.New("boom"),
	}
	err := runPrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "retrieving model version: boom")
}

func (*precheckBaseSuite) checkMachineVersionsMatch(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithMatchingTools())
	c.Assert(err, jc.ErrorIsNil)
}

func newBackendWithMatchingTools() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&machine{"0", version.MustParseBinary("1.2.3-trusty-amd64")},
			&machine{"1", version.MustParseBinary("1.2.3-xenial-amd64")},
		},
	}
}

func (*precheckBaseSuite) checkMachineVersionsDontMatch(c *gc.C, runPrecheck precheckRunner) {
	err := runPrecheck(newBackendWithMismatchingTools())
	c.Assert(err, gc.ErrorMatches, `machine 1 tools don't match model \(1.3.1 != 1.2.3\)`)
}

func newBackendWithMismatchingTools() *fakeBackend {
	return &fakeBackend{
		machines: []migration.PrecheckMachine{
			&machine{"0", version.MustParseBinary("1.2.3-trusty-amd64")},
			&machine{"1", version.MustParseBinary("1.3.1-xenial-amd64")},
		},
	}
}

type fakeBackend struct {
	cleanupNeeded bool
	cleanupErr    error

	agentVersionErr error

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

func (b *fakeBackend) AllMachines() ([]migration.PrecheckMachine, error) {
	return b.machines, b.allMachinesErr
}

func (b *fakeBackend) ControllerBackend() (migration.PrecheckBackend, error) {
	if b.controllerBackend == nil {
		return b, nil
	}
	return b.controllerBackend, nil
}

type machine struct {
	id      string
	version version.Binary
}

func (m *machine) Id() string {
	return m.id
}

func (m *machine) AgentTools() (*tools.Tools, error) {
	return &tools.Tools{
		Version: m.version,
	}, nil
}
