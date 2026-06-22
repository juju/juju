// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname_test

import (
	"testing"

	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/virtualhostname"
)

type HostnameSuite struct{}

func TestHostnameSuite(t *testing.T) {
	tc.Run(t, &HostnameSuite{})
}

func (s *HostnameSuite) TestParseContainerHostname(c *tc.C) {
	res, err := virtualhostname.Parse("charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, tc.Equals, "postgresql/1")
	c.Check(valid, tc.Equals, true)

	containerName, valid := res.Container()
	c.Check(containerName, tc.Equals, "charm")
	c.Check(valid, tc.Equals, true)

	_, valid = res.Machine()
	c.Check(valid, tc.Equals, false)

	c.Check(res.Target(), tc.Equals, virtualhostname.ContainerTarget)
	c.Check(res.ModelUUID(), tc.Equals, coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee"))
}

func (s *HostnameSuite) TestParseUnitHostname(c *tc.C) {
	res, err := virtualhostname.Parse("1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	unitName, valid := res.Unit()
	c.Check(unitName, tc.Equals, "postgresql/1")
	c.Check(valid, tc.Equals, true)

	_, valid = res.Container()
	c.Check(valid, tc.Equals, false)

	machineName, valid := res.Machine()
	c.Check(machineName, tc.Equals, coremachine.Name(""))
	c.Check(valid, tc.Equals, false)

	c.Check(res.Target(), tc.Equals, virtualhostname.UnitTarget)
	c.Check(res.ModelUUID(), tc.Equals, coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee"))
}

func (s *HostnameSuite) TestParseMachineHostname(c *tc.C) {
	res, err := virtualhostname.Parse("1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	_, valid := res.Unit()
	c.Check(valid, tc.Equals, false)

	_, valid = res.Container()
	c.Check(valid, tc.Equals, false)

	machineName, valid := res.Machine()
	c.Check(machineName, tc.Equals, coremachine.Name("1"))
	c.Check(valid, tc.Equals, true)

	c.Check(res.Target(), tc.Equals, virtualhostname.MachineTarget)
	c.Check(res.ModelUUID(), tc.Equals, coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee"))
}

func (s *HostnameSuite) TestParseNestedMachineHostname(c *tc.C) {
	res, err := virtualhostname.Parse("1-lxd-0.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, tc.IsNil)

	_, valid := res.Unit()
	c.Check(valid, tc.Equals, false)

	_, valid = res.Container()
	c.Check(valid, tc.Equals, false)

	machineName, valid := res.Machine()
	c.Check(machineName, tc.Equals, coremachine.Name("1/lxd/0"))
	c.Check(valid, tc.Equals, true)

	c.Check(res.Target(), tc.Equals, virtualhostname.MachineTarget)
	c.Check(res.ModelUUID(), tc.Equals, coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee"))
}
