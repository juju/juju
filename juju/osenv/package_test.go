// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type importSuite struct {
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (*importSuite) TestDependencies(c *tc.C) {
	c.Assert(coretesting.FindJujuCoreImports(c, "github.com/juju/juju/juju/osenv"), tc.SameContents, []string{
		"internal/featureflag",
	})
}
