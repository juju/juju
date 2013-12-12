// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

func Test(t *testing.T) { gc.TestingT(t) }

type Dependencies struct{}

var _ = gc.Suite(&Dependencies{})

func (*Dependencies) TestPackageDependencies(c *gc.C) {
	// This test is to ensure we don't bring in dependencies on state, environ
	// or any of the other bigger packages that'll drag in yet more dependencies.
	// Only imports that start with "launchpad.net/juju-core" are checked, and the
	// resulting slice has that prefix removed to keep the output short.
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/cmd"),
		gc.DeepEquals,
		[]string{"environs/config", "juju/osenv", "log", "names", "version"})
}
