// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/v2/testing"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/v2/core/settings")

	// This package only brings in other core packages.
	c.Assert(found, jc.SameContents, []string{})
}
