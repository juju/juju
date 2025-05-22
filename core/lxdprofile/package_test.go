// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)


type ImportTest struct{}

func TestImportTest(t *stdtesting.T) { tc.Run(t, &ImportTest{}) }
func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/lxdprofile")

	// This package brings in nothing else from juju/juju
	c.Assert(found, tc.SameContents, []string{
		"core/errors",
		"internal/errors",
	})
}
