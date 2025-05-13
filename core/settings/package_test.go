// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings_test

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type importSuite struct{}

var _ = tc.Suite(&importSuite{})

func (*importSuite) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/settings")

	// This package only brings in other core packages.
	c.Assert(found, tc.SameContents, []string{
		"core/arch",
		"core/semversion",
		"internal/charm",
		"internal/charm/assumes",
		"internal/charm/hooks",
		"internal/charm/resource",
		"internal/errors",
	})
}
