// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environPolSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environPolSuite{})

func (*environPolSuite) TestPrecheckInstance(c *gc.C) {
}

func (*environPolSuite) TestSupportedArchitectures(c *gc.C) {
}

func (*environPolSuite) TestConstraintsValidator(c *gc.C) {
}

func (*environPolSuite) TestSupportNetworks(c *gc.C) {
}

func (*environPolSuite) TestSupportAddressAllocation(c *gc.C) {
}
