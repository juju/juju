// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type ImportTest struct{}

var _ = tc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/migration")

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, tc.SameContents, []string{
		"core/credential",
		"core/errors",
		"core/life",
		"core/logger",
		"core/model",
		"core/network",
		"core/permission",
		"core/resource",
		"core/semversion",
		"core/status",
		"core/trace",
		"core/unit",
		"core/user",
		"internal/charm/resource",
		"internal/errors",
		"internal/logger",
		"internal/uuid",
	})
}
