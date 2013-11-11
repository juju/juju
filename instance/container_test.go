// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type InstanceSuite struct{}

var _ = gc.Suite(&InstanceSuite{})

func (s *InstanceSuite) TestParseAllowedContainerType(c *gc.C) {
	ctype, err := instance.ParseAllowedContainerType("lxc")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.ContainerType("lxc"))
	ctype, err = instance.ParseAllowedContainerType("none")
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *InstanceSuite) TestParseAllowedContainerTypeOrNone(c *gc.C) {
	ctype, err := instance.ParseAllowedContainerTypeOrNone("lxc")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.ContainerType("lxc"))
	ctype, err = instance.ParseAllowedContainerTypeOrNone("none")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.ContainerType("none"))
}
