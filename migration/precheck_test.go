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

// Ensure PrecheckShim matches the required interface.
var _ migration.PrecheckBackend = new(migration.PrecheckShim)

type SourcePrecheckSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&SourcePrecheckSuite{})

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

func (*SourcePrecheckSuite) TestAgentVersionError(c *gc.C) {
	backend := &fakeBackend{
		agentVersionErr: errors.New("boom"),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "retrieving model version: boom")
}

func (*SourcePrecheckSuite) TestMachineVersionsMatch(c *gc.C) {
	backend := &fakeBackend{
		machines: []migration.PrecheckMachine{
			&machine{"0", version.MustParseBinary("1.2.3-trusty-amd64")},
			&machine{"1", version.MustParseBinary("1.2.3-xenial-amd64")},
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (*SourcePrecheckSuite) TestMachineVersionsDontMatch(c *gc.C) {
	backend := &fakeBackend{
		machines: []migration.PrecheckMachine{
			&machine{"0", version.MustParseBinary("1.2.3-trusty-amd64")},
			&machine{"1", version.MustParseBinary("1.3.1-xenial-amd64")},
		},
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, `machine 1 tools don't match model \(1.3.1 != 1.2.3\)`)
}

type fakeBackend struct {
	cleanupNeeded bool
	cleanupErr    error

	agentVersionErr error

	machines       []migration.PrecheckMachine
	allMachinesErr error
}

func (b *fakeBackend) NeedsCleanup() (bool, error) {
	return b.cleanupNeeded, b.cleanupErr
}

func (b *fakeBackend) AgentVersion() (version.Number, error) {
	return version.MustParse("1.2.3"), b.agentVersionErr
}

func (b *fakeBackend) AllMachines() ([]migration.PrecheckMachine, error) {
	return b.machines, b.allMachinesErr
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
