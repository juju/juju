// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportSuite struct{}

var _ = tc.Suite(&ImportSuite{})

var allowedCoreImports = set.NewStrings(
	"core/credential",
	"core/errors",
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

func (*ImportSuite) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/network")

	c.Assert(found, jc.SameContents, allowedCoreImports.SortedValues())
}
