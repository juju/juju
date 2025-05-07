// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/virtualhostname"
)

type HostnameSuite struct{}

var _ = tc.Suite(&HostnameSuite{})

func (s *HostnameSuite) TestParseContainerHostname(c *tc.C) {
	res, err := virtualhostname.Parse("charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, tc.Equals, "postgresql/1")
	c.Check(valid, tc.Equals, true)

	containerName, valid := res.Container()
	c.Check(containerName, tc.Equals, "charm")
	c.Check(valid, tc.Equals, true)

	machineNumber, valid := res.Machine()
	c.Check(machineNumber, tc.Equals, 0)
	c.Check(valid, tc.Equals, false)

	c.Check(res.Target(), tc.Equals, virtualhostname.ContainerTarget)
	c.Check(res.ModelUUID(), tc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
}

func (s *HostnameSuite) TestParseUnitHostname(c *tc.C) {
	res, err := virtualhostname.Parse("1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, tc.Equals, "postgresql/1")
	c.Check(valid, tc.Equals, true)

	containerName, valid := res.Container()
	c.Check(containerName, tc.Equals, "")
	c.Check(valid, tc.Equals, false)

	machineNumber, valid := res.Machine()
	c.Check(machineNumber, tc.Equals, 0)
	c.Check(valid, tc.Equals, false)

	c.Check(res.Target(), tc.Equals, virtualhostname.UnitTarget)
	c.Check(res.ModelUUID(), tc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
}

func (s *HostnameSuite) TestParseMachineHostname(c *tc.C) {
	res, err := virtualhostname.Parse("1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, tc.Equals, "/0")
	c.Check(valid, tc.Equals, false)

	containerName, valid := res.Container()
	c.Check(containerName, tc.Equals, "")
	c.Check(valid, tc.Equals, false)

	machineNumber, valid := res.Machine()
	c.Check(machineNumber, tc.Equals, 1)
	c.Check(valid, tc.Equals, true)

	c.Check(res.Target(), tc.Equals, virtualhostname.MachineTarget)
	c.Check(res.ModelUUID(), tc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
}
