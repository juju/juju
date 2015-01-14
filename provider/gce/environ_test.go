// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (*environSuite) TestName(c *gc.C) {
}

func (*environSuite) TestProvider(c *gc.C) {
}

func (*environSuite) TestRegion(c *gc.C) {
}

func (*environSuite) TestCloudSpec(c *gc.C) {
}

func (*environSuite) TestSetConfig(c *gc.C) {
}

func (*environSuite) TestConfig(c *gc.C) {
}

func (*environSuite) TestBootstrap(c *gc.C) {
}

func (*environSuite) TestDestroy(c *gc.C) {
}
