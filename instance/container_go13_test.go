// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package instance_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
)

func (s *InstanceSuite) TestParseContainerTypeLXD(c *gc.C) {
	ctype, err := instance.ParseContainerType("lxd")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctype, gc.Equals, instance.LXD)
}

func (s *InstanceSuite) TestParseContainerTypeLXDOrNone(c *gc.C) {
	ctype, err := instance.ParseContainerTypeOrNone("lxd")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctype, gc.Equals, instance.LXD)
}
