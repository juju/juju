// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

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
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/lxdprofile")

	// This package brings in nothing else from juju/juju
	c.Assert(found, jc.SameContents, []string{
		"core/errors",
		"internal/errors",
	})
}
