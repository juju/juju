// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	stdtesting "testing"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type importSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestDependencies(c *gc.C) {
	// TODO (frankban): restore this test once juju-core/utils is on Github.
	c.Skip("waiting for juju-core/utils to be migrated to github")
	// This test is to ensure we don't bring in dependencies at all.
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/juju/osenv"),
		gc.HasLen, 0)
}

// TODO (frankban): remove this test once juju-core/utils is on Github.
func (*importSuite) TestTemporaryDependencies(c *gc.C) {
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/juju/osenv"),
		gc.DeepEquals, []string{"utils"})
}
