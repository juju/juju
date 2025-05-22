// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)


type importSuite struct{}

func TestImportSuite(t *stdtesting.T) { tc.Run(t, &importSuite{}) }
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
