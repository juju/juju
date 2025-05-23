// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportTest struct{}

func TestImportTest(t *testing.T) {
	tc.Run(t, &ImportTest{})
}

func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/constraints")

	// This package should only depend on the core packages and the utils/stringcompare package.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, tc.SameContents, []string{
		"core/arch",
		"core/errors",
		"core/instance",
		"core/status",
		"internal/errors",
		"internal/stringcompare",
	})
}
