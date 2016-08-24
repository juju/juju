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

type PrecheckSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&PrecheckSuite{})

func (*PrecheckSuite) TestPrecheckCleanups(c *gc.C) {
	backend := &fakePrecheckBackend{}
	err := migration.Precheck(backend)
	c.Assert(err, jc.ErrorIsNil)
}

func (*PrecheckSuite) TestPrecheckCleanupsError(c *gc.C) {
	backend := &fakePrecheckBackend{
		cleanupError: errors.New("boom"),
	}
	err := migration.Precheck(backend)
	c.Assert(err, gc.ErrorMatches, "precheck cleanups: boom")
}

func (*PrecheckSuite) TestPrecheckCleanupsNeeded(c *gc.C) {
	backend := &fakePrecheckBackend{
		cleanupNeeded: true,
	}
	err := migration.Precheck(backend)
	c.Assert(err, gc.ErrorMatches, "precheck failed: cleanup needed")
}

type fakePrecheckBackend struct {
	cleanupNeeded bool
	cleanupError  error
}

func (f *fakePrecheckBackend) NeedsCleanup() (bool, error) {
	return f.cleanupNeeded, f.cleanupError
}
