// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ImportTest struct{}

func TestImportTest(t *stdtesting.T) { tc.Run(t, &ImportTest{}) }
func (*ImportTest) TestImports(c *tc.C) {
	// TODO (stickupkid): There is a circular dependency between the user
	// package and the testing package, caused by the model package.
	//
	// This breaks the link for now.
	const jujuPkgPrefix = "github.com/juju/juju/"

	found, err := testhelpers.FindImports("github.com/juju/juju/core/user", jujuPkgPrefix)
	c.Assert(err, tc.ErrorIsNil)

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, tc.SameContents, []string{
		"core/errors",
		"internal/errors",
		"internal/uuid",
	})
}
