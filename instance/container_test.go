// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	"testing"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

func TestPackage(t *testing.T) {
	TestingT(t)
}

type InstanceSuite struct{}

var _ = Suite(&InstanceSuite{})

func (s *InstanceSuite) TestParseSupportedContainerType(c *C) {
	ctype, err := instance.ParseSupportedContainerType("lxc")
	c.Assert(err, IsNil)
	c.Assert(ctype, Equals, instance.ContainerType("lxc"))
	ctype, err = instance.ParseSupportedContainerType("none")
	c.Assert(err, Not(IsNil))
}

func (s *InstanceSuite) TestParseSupportedContainerTypeOrNone(c *C) {
	ctype, err := instance.ParseSupportedContainerTypeOrNone("lxc")
	c.Assert(err, IsNil)
	c.Assert(ctype, Equals, instance.ContainerType("lxc"))
	ctype, err = instance.ParseSupportedContainerTypeOrNone("none")
	c.Assert(err, IsNil)
	c.Assert(ctype, Equals, instance.ContainerType("none"))
}
