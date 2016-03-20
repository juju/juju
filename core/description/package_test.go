// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

// Useful test constants.

// Constraints and CloudInstance store megabytes
const gig uint64 = 1024

// None of the tests in this package require mongo.
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/description")

	// This package brings in nothing else from juju/juju
	c.Assert(found, gc.HasLen, 0)
}
