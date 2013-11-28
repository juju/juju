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

func (s *InstanceSuite) TestParseContainerType(c *gc.C) {
	ctype, err := instance.ParseContainerType("lxc")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.LXC)

	ctype, err = instance.ParseContainerType("kvm")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.KVM)

	ctype, err = instance.ParseContainerType("none")
	c.Assert(err, gc.ErrorMatches, `invalid container type "none"`)

	ctype, err = instance.ParseContainerType("omg")
	c.Assert(err, gc.ErrorMatches, `invalid container type "omg"`)
}

func (s *InstanceSuite) TestParseContainerTypeOrNone(c *gc.C) {
	ctype, err := instance.ParseContainerTypeOrNone("lxc")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.LXC)

	ctype, err = instance.ParseContainerTypeOrNone("kvm")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.KVM)

	ctype, err = instance.ParseContainerTypeOrNone("none")
	c.Assert(err, gc.IsNil)
	c.Assert(ctype, gc.Equals, instance.NONE)

	ctype, err = instance.ParseContainerTypeOrNone("omg")
	c.Assert(err, gc.ErrorMatches, `invalid container type "omg"`)
}
