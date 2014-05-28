// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
)

type importSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestDependencies(c *gc.C) {
	// This test is to ensure we don't bring in dependencies at all.
	c.Assert(coretesting.FindJujuCoreImports(c, "launchpad.net/juju-core/utils"),
		gc.HasLen, 0)
}
