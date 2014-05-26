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

// TODO (frankban): remove this test once juju-core/utils is on Github.
func (*importSuite) TestTemporaryDependencies(c *gc.C) {
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/juju/osenv"),
		gc.DeepEquals, []string{"utils"})
}
