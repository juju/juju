// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type instanceSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (*instanceSuite) TestNewInstance(c *gc.C) {
}

func (*instanceSuite) TestID(c *gc.C) {
}

func (*instanceSuite) TestStatus(c *gc.C) {
}

func (*instanceSuite) TestRefresh(c *gc.C) {
}

func (*instanceSuite) TestAddresses(c *gc.C) {
}

func (*instanceSuite) TestOpenPorts(c *gc.C) {
}

func (*instanceSuite) TestClosePorts(c *gc.C) {
}

func (*instanceSuite) TestPorts(c *gc.C) {
}
