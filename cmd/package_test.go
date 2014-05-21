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
	// This test is to ensure we don't bring in dependencies without thinking.
	// Looking at the "environs/config", it is just for JujuHome.  This should
	// really be moved into "juju/osenv".
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/cmd"),
		gc.DeepEquals,
		[]string{"juju/arch", "juju/osenv", "names", "utils", "version"})
}
