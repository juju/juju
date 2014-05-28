// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type DependencySuite struct{}

var _ = gc.Suite(&DependencySuite{})

func (*DependencySuite) TestPackageDependencies(c *gc.C) {
	// This test is to ensure we don't bring in any juju-core dependencies.
	c.Assert(testing.FindJujuCoreImports(c, "launchpad.net/juju-core/testing/testbase"),
		gc.HasLen, 0)
}
