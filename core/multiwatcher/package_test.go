// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/multiwatcher")

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, jc.SameContents, []string{
		"core/arch",
		"core/constraints",
		"core/credential",
		"core/instance",
		"core/life",
		"core/logger",
		"core/model",
		"core/network",
		"core/permission",
		"core/status",
		"core/user",
		"internal/uuid",
		"internal/logger",
	})
}
