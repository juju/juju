// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/internal/testing"
)

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}

type importSuite struct {
}

var _ = tc.Suite(&importSuite{})

func (*importSuite) TestDependencies(c *tc.C) {
	c.Assert(coretesting.FindJujuCoreImports(c, "github.com/juju/juju/juju/osenv"), jc.SameContents, []string{
		"internal/featureflag",
	})
}
