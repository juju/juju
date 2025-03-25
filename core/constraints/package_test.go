// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/constraints")

	// This package should only depend on the core packages and the utils/stringcompare package.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, jc.SameContents, []string{
		"core/arch",
		"core/errors",
		"core/instance",
		"core/status",
		"internal/errors",
		"internal/stringcompare",
	})
}
