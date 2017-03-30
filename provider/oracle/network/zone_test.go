// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/juju/provider/oracle/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type zoneSuite struct{}

var _ = gc.Suite(&zoneSuite{})

func (z *zoneSuite) TestNewAvailabilityZone(c *gc.C) {
	name := "us6"
	zone := network.NewAvailabilityZone(name)
	c.Assert(zone, gc.NotNil)
	c.Assert(zone.Available(), jc.IsTrue)
	c.Assert(zone.Name(), jc.DeepEquals, name)
}
