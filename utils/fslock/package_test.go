// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fslock_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) { gc.TestingT(t) }

type Dependencies struct{}

var _ = gc.Suite(&Dependencies{})

func (*Dependencies) TestPackageDependencies(c *gc.C) {
	// This test is to ensure we don't bring in dependencies without thinking.
	c.Assert(coretesting.FindJujuCoreImports(c, "launchpad.net/juju-core/utils/fslock"),
		gc.DeepEquals, []string{"utils"})
}
