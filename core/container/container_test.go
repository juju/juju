// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
)

type ContainerSuite struct{}

var _ = gc.Suite(&ContainerSuite{})

func (s *ContainerSuite) TestNestingLevel(c *gc.C) {
	c.Assert(container.NestingLevel("0"), gc.Equals, 0)
	c.Assert(container.NestingLevel("0/lxd/1"), gc.Equals, 1)
	c.Assert(container.NestingLevel("0/lxd/1/kvm/0"), gc.Equals, 2)
}

func (s *ContainerSuite) TestTopParentId(c *gc.C) {
	c.Assert(container.TopParentId("0"), gc.Equals, "0")
	c.Assert(container.TopParentId("0/lxd/1"), gc.Equals, "0")
	c.Assert(container.TopParentId("0/lxd/1/kvm/2"), gc.Equals, "0")
}

func (s *ContainerSuite) TestParentId(c *gc.C) {
	c.Assert(container.ParentId("0"), gc.Equals, "")
	c.Assert(container.ParentId("0/lxd/1"), gc.Equals, "0")
	c.Assert(container.ParentId("0/lxd/1/kvm/0"), gc.Equals, "0/lxd/1")
}

func (s *ContainerSuite) TestContainerTypeFromId(c *gc.C) {
	c.Assert(container.ContainerTypeFromId("0"), gc.Equals, instance.ContainerType(""))
	c.Assert(container.ContainerTypeFromId("0/lxd/1"), gc.Equals, instance.LXD)
}
