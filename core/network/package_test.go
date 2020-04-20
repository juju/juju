// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

//go:generate mockgen -package network -destination package_mock_test.go github.com/juju/juju/core/network LinkLayerDevice,LinkLayerDeviceAddress

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/network")

	// This package only brings in other core packages.
	c.Assert(found, jc.SameContents, []string{})
}
