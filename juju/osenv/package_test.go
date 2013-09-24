// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestDependencies(c *gc.C) {
	// This test is to ensure we don't bring in dependencies at all.
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/juju/osenv"),
		gc.HasLen, 0)
}
