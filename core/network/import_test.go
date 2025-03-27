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
	"core/credential",
	"core/life",
	"core/logger",
	"core/model",
	"core/permission",
	"core/semversion",
	"core/status",
	"core/trace",
	"core/user",
	"internal/errors",
	"internal/logger",
	"internal/uuid",
)

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/network")

	c.Assert(found, jc.SameContents, allowedCoreImports.SortedValues())
}
