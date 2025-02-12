// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
)

type HostnameSuite struct{}

var _ = gc.Suite(&HostnameSuite{})

func (s *HostnameSuite) TestParseContainerHostname(c *gc.C) {
	res, err := virtualhostname.Parse("charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, gc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, gc.Equals, "postgresql/1")
	c.Check(valid, gc.Equals, true)

	containerName, valid := res.Container()
	c.Check(containerName, gc.Equals, "charm")
	c.Check(valid, gc.Equals, true)

	machineNumber, valid := res.Machine()
	c.Check(machineNumber, gc.Equals, 0)
	c.Check(valid, gc.Equals, false)

	c.Check(res.Target(), gc.Equals, virtualhostname.ContainerTarget)
	c.Check(res.ModelUUID(), gc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
}

func (s *HostnameSuite) TestParseUnitHostname(c *gc.C) {
	res, err := virtualhostname.Parse("1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, gc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, gc.Equals, "postgresql/1")
	c.Check(valid, gc.Equals, true)

	containerName, valid := res.Container()
	c.Check(containerName, gc.Equals, "")
	c.Check(valid, gc.Equals, false)

	machineNumber, valid := res.Machine()
	c.Check(machineNumber, gc.Equals, 0)
	c.Check(valid, gc.Equals, false)

	c.Check(res.Target(), gc.Equals, virtualhostname.UnitTarget)
	c.Check(res.ModelUUID(), gc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
}

func (s *HostnameSuite) TestParseMachineHostname(c *gc.C) {
	res, err := virtualhostname.Parse("1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, gc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, gc.Equals, "")
	c.Check(valid, gc.Equals, false)

	containerName, valid := res.Container()
	c.Check(containerName, gc.Equals, "")
	c.Check(valid, gc.Equals, false)

	machineNumber, valid := res.Machine()
	c.Check(machineNumber, gc.Equals, 1)
	c.Check(valid, gc.Equals, true)

	c.Check(res.Target(), gc.Equals, virtualhostname.MachineTarget)
	c.Check(res.ModelUUID(), gc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
}
