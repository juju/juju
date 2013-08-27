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

func (s *InstanceSuite) TestParseSupportedContainerType(c *gc.C) {
	ctype, err := instance.ParseSupportedContainerType("lxc")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.ContainerType("lxc"))
	ctype, err = instance.ParseSupportedContainerType("none")
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *InstanceSuite) TestParseSupportedContainerTypeOrNone(c *gc.C) {
	ctype, err := instance.ParseSupportedContainerTypeOrNone("lxc")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.ContainerType("lxc"))
	ctype, err = instance.ParseSupportedContainerTypeOrNone("none")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.ContainerType("none"))
}
