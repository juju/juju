// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

func Test(t *testing.T) {
	tc.TestingT(t)
}

type ImportTest struct{}

var _ = tc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/status")
	c.Check(found, tc.SameContents, []string{
		"core/errors",
		"internal/errors",
	})
}
