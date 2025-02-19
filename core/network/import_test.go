// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

var allowedCoreImports = set.NewStrings(
	"core/life",
	"core/logger",
	"core/trace",
	"internal/errors",
	"internal/logger",
)

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/network")
	for _, packageImport := range found {
		c.Assert(allowedCoreImports.Contains(packageImport), jc.IsTrue)
	}
}
