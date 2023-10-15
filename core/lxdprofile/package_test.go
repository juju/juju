// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/lxdprofile")

	// This package brings in nothing else from juju/juju
	c.Assert(found, gc.HasLen, 0)
}
