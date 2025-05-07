// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"testing"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type ImportTest struct{}

var _ = tc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *tc.C) {
	// TODO (stickupkid): There is a circular dependency between the user
	// package and the testing package, caused by the model package.
	//
	// This breaks the link for now.
	const jujuPkgPrefix = "github.com/juju/juju/"

	found, err := jujutesting.FindImports("github.com/juju/juju/core/user", jujuPkgPrefix)
	c.Assert(err, jc.ErrorIsNil)

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, jc.SameContents, []string{
		"core/errors",
		"internal/errors",
		"internal/uuid",
	})
}
