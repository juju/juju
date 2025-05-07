// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type ImportTest struct{}

var _ = tc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/base")
	c.Assert(found, jc.SameContents, []string{
		"core/arch",
		"core/errors",
		"internal/charm",
		"internal/charm/assumes",
		"internal/charm/hooks",
		"internal/charm/resource",
		"core/semversion",
		"internal/errors",
	})
}
