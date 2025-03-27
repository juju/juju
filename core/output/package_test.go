// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/internal/testing"
)

// None of the tests in this package require mongo.
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/output")

	c.Assert(found, jc.SameContents, []string{
		"core/credential",
		"core/life",
		"core/logger",
		"core/model",
		"core/permission",
		"core/semversion",
		"core/status",
		"core/trace",
		"core/user",
		"internal/cmd",
		"internal/errors",
		"internal/logger",
		"internal/stringcompare",
		"internal/uuid",
	})
}
