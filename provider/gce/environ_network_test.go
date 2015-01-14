// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environNetSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environNetSuite{})

func (*environNetSuite) TestGlobalFirewallName(c *gc.C) {
}

func (*environNetSuite) TestOpenPorts(c *gc.C) {
}

func (*environNetSuite) TestClosePorts(c *gc.C) {
}

func (*environNetSuite) TestPorts(c *gc.C) {
}
