// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportSuite struct{}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &ImportSuite{})
}

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

	c.Assert(found, tc.SameContents, allowedCoreImports.SortedValues())
}
