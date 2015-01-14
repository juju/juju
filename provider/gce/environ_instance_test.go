// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce"
)

type environInstSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environInstSuite{})

func (*environInstSuite) TestInstances(c *gc.C) {
}

func (*environInstSuite) TestBasicInstances(c *gc.C) {
}

func (*environInstSuite) TestStateServerInstances(c *gc.C) {
}

func (*environInstSuite) TestParsePlacement(c *gc.C) {
}

func (*environInstSuite) TestCheckInstanceType(c *gc.C) {
}
