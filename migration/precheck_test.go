// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/migration"
	"github.com/juju/juju/testing"
)

type SourcePrecheckSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&SourcePrecheckSuite{})

func (*SourcePrecheckSuite) TestCleanups(c *gc.C) {
	backend := &fakePrecheckBackend{}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (*SourcePrecheckSuite) TestCleanupsError(c *gc.C) {
	backend := &fakePrecheckBackend{
		cleanupError: errors.New("boom"),
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "checking cleanups: boom")
}

func (*SourcePrecheckSuite) TestCleanupsNeeded(c *gc.C) {
	backend := &fakePrecheckBackend{
		cleanupNeeded: true,
	}
	err := migration.SourcePrecheck(backend)
	c.Assert(err, gc.ErrorMatches, "precheck failed: cleanup needed")
}

type fakePrecheckBackend struct {
	cleanupNeeded bool
	cleanupError  error
}

func (b *fakePrecheckBackend) NeedsCleanup() (bool, error) {
	return b.cleanupNeeded, b.cleanupError
}
