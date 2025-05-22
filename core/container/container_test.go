// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
)

type ContainerSuite struct{}

func TestContainerSuite(t *stdtesting.T) {
	tc.Run(t, &ContainerSuite{})
}

func (s *ContainerSuite) TestNestingLevel(c *tc.C) {
	c.Assert(container.NestingLevel("0"), tc.Equals, 0)
	c.Assert(container.NestingLevel("0/lxd/1"), tc.Equals, 1)
	c.Assert(container.NestingLevel("0/lxd/1/kvm/0"), tc.Equals, 2)
}

func (s *ContainerSuite) TestTopParentId(c *tc.C) {
	c.Assert(container.TopParentId("0"), tc.Equals, "0")
	c.Assert(container.TopParentId("0/lxd/1"), tc.Equals, "0")
	c.Assert(container.TopParentId("0/lxd/1/kvm/2"), tc.Equals, "0")
}

func (s *ContainerSuite) TestParentId(c *tc.C) {
	c.Assert(container.ParentId("0"), tc.Equals, "")
	c.Assert(container.ParentId("0/lxd/1"), tc.Equals, "0")
	c.Assert(container.ParentId("0/lxd/1/kvm/0"), tc.Equals, "0/lxd/1")
}

func (s *ContainerSuite) TestContainerTypeFromId(c *tc.C) {
	c.Assert(container.ContainerTypeFromId("0"), tc.Equals, instance.ContainerType(""))
	c.Assert(container.ContainerTypeFromId("0/lxd/1"), tc.Equals, instance.LXD)
}
