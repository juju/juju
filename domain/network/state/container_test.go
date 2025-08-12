// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/charm"
)

type containerSuite struct {
	linkLayerBaseSuite
}

func TestContainerSuite(t *testing.T) {
	tc.Run(t, &containerSuite{})
}

func (s *containerSuite) TestGetMachineSpaceConstraints(c *tc.C) {
	db := s.DB()

	// Arrange. Set up two spaces and a machine with those as
	// positive and negative constraints respectively.
	nUUID := s.addNetNode(c)
	mUUID := s.addMachine(c, "0", nUUID)
	posSpace := s.addSpace(c)
	negSpace := s.addSpace(c)
	conUUID := "constraint-uuid"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, `INSERT INTO "constraint" (uuid) VALUES (?)`, conUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(ctx, "INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)",
		mUUID, conUUID)
	c.Assert(err, tc.ErrorIsNil)

	for i, s := range []string{posSpace, negSpace} {
		exclude := i != 0

		_, err := db.ExecContext(ctx, "INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)",
			conUUID, s, exclude)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Act
	pos, neg, err := s.state.GetMachineSpaceConstraints(ctx, mUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Assert(pos, tc.HasLen, 1)
	c.Assert(neg, tc.HasLen, 1)
	c.Check(pos[0].UUID, tc.Equals, posSpace)
	c.Check(neg[0].UUID, tc.Equals, negSpace)
}

func (s *containerSuite) TestGetMachineAppBindingsBoundEndpoints(c *tc.C) {
	db := s.DB()

	// Arrange. Set up a unit of an application with a bound endpoint,
	// assigned to a machine. Ensure the machine has a NIC connected
	// to the bound space.
	cUUID := s.addCharm(c)
	rUUID := s.addCharmRelation(c, cUUID, charm.Relation{
		Name:  "whatever",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	})

	spUUID := s.addSpace(c)
	subUUID := s.addSubnet(c, "192.168.10.0/24", spUUID)

	appUUID := s.addApplication(c, cUUID, "app1")
	_ = s.addApplicationEndpoint(c, appUUID, rUUID, spUUID)

	nUUID := s.addNetNode(c)
	mUUID := s.addMachine(c, "0", nUUID)
	_ = s.addUnit(c, "app1/0", appUUID, cUUID, nUUID)

	dUUID := s.addLinkLayerDevice(c, nUUID, "eth0", "mac-address", corenetwork.EthernetDevice)
	addrUUID := s.addIPAddress(c, dUUID, nUUID, "192.168.10.10/24", 0)

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "UPDATE ip_address SET subnet_uuid = ? WHERE uuid = ?", subUUID, addrUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	bound, err := s.state.GetMachineAppBindings(ctx, mUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bound, tc.HasLen, 1)
	c.Check(bound[0].UUID, tc.Equals, spUUID)
}

func (s *containerSuite) TestGetMachineAppBindingsDefaultBinding(c *tc.C) {
	db := s.DB()

	// Arrange. Set up a unit of an application with a non-bound endpoint,
	// but *with* a non-alpha default binding, assigned to a machine.
	// Ensure the machine has a NIC connected to the bound space.
	cUUID := s.addCharm(c)
	rUUID := s.addCharmRelation(c, cUUID, charm.Relation{
		Name:  "whatever",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	})

	spUUID := s.addSpace(c)
	subUUID := s.addSubnet(c, "192.168.10.0/24", spUUID)

	appUUID := s.addApplication(c, cUUID, "app1")
	_ = s.addApplicationEndpoint(c, appUUID, rUUID, "")

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "UPDATE application SET space_uuid = ?", spUUID)
	c.Assert(err, tc.ErrorIsNil)

	nUUID := s.addNetNode(c)
	mUUID := s.addMachine(c, "0", nUUID)
	_ = s.addUnit(c, "app1/0", appUUID, cUUID, nUUID)

	dUUID := s.addLinkLayerDevice(c, nUUID, "eth0", "mac-address", corenetwork.EthernetDevice)
	addrUUID := s.addIPAddress(c, dUUID, nUUID, "192.168.10.10/24", 0)

	_, err = db.ExecContext(ctx, "UPDATE ip_address SET subnet_uuid = ? WHERE uuid = ?", subUUID, addrUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	bound, err := s.state.GetMachineAppBindings(ctx, mUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bound, tc.HasLen, 1)
	c.Check(bound[0].UUID, tc.Equals, spUUID)
}

func (s *containerSuite) TestNICsInSpaces(c *tc.C) {
	db := s.DB()

	// Arrange. Add a device in 2 spaces, and another in none.
	spUUID1 := s.addSpace(c)
	subUUID1 := s.addSubnet(c, "192.168.10.0/24", spUUID1)

	spUUID2 := s.addSpace(c)
	subUUID2 := s.addSubnet(c, "192.168.20.0/24", spUUID2)

	nUUID := s.addNetNode(c)
	eth := "eth0"
	bond := "bond0"
	ethMAC := "eth-mac-address"
	bondMAC := "bond-mac-address"
	dUUID1 := s.addLinkLayerDevice(c, nUUID, eth, ethMAC, corenetwork.EthernetDevice)
	_ = s.addLinkLayerDevice(c, nUUID, bond, bondMAC, corenetwork.BondDevice)

	addrUUID1 := s.addIPAddress(c, dUUID1, nUUID, "192.168.10.10/24", 0)
	addrUUID2 := s.addIPAddress(c, dUUID1, nUUID, "192.168.20.20/24", 0)

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "UPDATE ip_address SET subnet_uuid = ? WHERE uuid = ?", subUUID1, addrUUID1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(ctx, "UPDATE ip_address SET subnet_uuid = ? WHERE uuid = ?", subUUID2, addrUUID2)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(ctx, "UPDATE link_layer_device SET virtual_port_type_id = 1 WHERE uuid = ?", dUUID1)
	c.Assert(err, tc.ErrorIsNil)

	// Act.
	nics, err := s.state.NICsInSpaces(ctx, nUUID)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.DeepEquals, map[string][]network.NetInterface{
		spUUID1: {{
			Name:            eth,
			MACAddress:      &ethMAC,
			Type:            corenetwork.EthernetDevice,
			VirtualPortType: corenetwork.OvsPort,
		}},
		spUUID2: {{
			Name:            eth,
			MACAddress:      &ethMAC,
			Type:            corenetwork.EthernetDevice,
			VirtualPortType: corenetwork.OvsPort,
		}},
		"": {{
			Name:            bond,
			MACAddress:      &bondMAC,
			Type:            corenetwork.BondDevice,
			VirtualPortType: corenetwork.NonVirtualPort,
		}},
	})

}

func (s *containerSuite) TestGetSubnetCIDRForDevice(c *tc.C) {
	db := s.DB()

	// Arrange. Add a device with an IP address in a subnet.
	cidr := "10.10.10.0/24"
	spUUID := s.addSpace(c)
	subUUID := s.addSubnet(c, cidr, spUUID)

	nUUID := s.addNetNode(c)

	devName := "eth0"
	dUUID := s.addLinkLayerDevice(c, nUUID, devName, "mac-address", corenetwork.EthernetDevice)
	addrUUID := s.addIPAddress(c, dUUID, nUUID, "10.10.10.100/24", 0)

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "UPDATE ip_address SET subnet_uuid = ? WHERE uuid = ?", subUUID, addrUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act.
	gotCIDR, err := s.state.GetSubnetCIDRForDevice(ctx, nUUID, devName, spUUID)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotCIDR, tc.Equals, cidr)
}

func (s *containerSuite) TestGetSubnetCIDRForDeviceNotFound(c *tc.C) {

	// Arrange. Add a device with no address.
	// Without an address on this device, we can't locate a subnet.
	nUUID := s.addNetNode(c)

	devName := "eth0"
	_ = s.addLinkLayerDevice(c, nUUID, devName, "mac-address", corenetwork.EthernetDevice)

	// Act.
	_, err := s.state.GetSubnetCIDRForDevice(c.Context(), nUUID, devName, s.addSpace(c))

	// Assert.
	c.Assert(err, tc.ErrorIs, errors.SubnetNotFound)
}

func (s *containerSuite) TestGetContainerNetworkingMethod(c *tc.C) {
	db := s.DB()

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO model_config VALUES ('container-networking-method', 'provider')")
	c.Assert(err, tc.ErrorIsNil)

	conf, err := s.state.GetContainerNetworkingMethod(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conf, tc.Equals, "provider")
}

// addCharm inserts a new charm into the database and returns the UUID.
func (s *containerSuite) addCharm(c *tc.C) corecharm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
	return charmUUID
}

// addApplication adds a new application to the database with the specified
// charm UUID and application name. It returns the application UUID.
func (s *containerSuite) addApplication(c *tc.C, charmUUID corecharm.ID, appName string) coreapplication.ID {
	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID.String(), corenetwork.AlphaSpaceId)
	return appUUID
}

// addUnit adds a new unit to the specified application in the database with
// the given UUID and name. Returns the unit uuid.
func (s *containerSuite) addUnit(
	c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID, charmUUID corecharm.ID, nodeUUID string,
) coreunit.UUID {
	unitUUID := coreunittesting.GenUnitUUID(c)
	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES (?, ?, ?, ?, ?, ?)
`, unitUUID, unitName, 0 /* alive */, appUUID, charmUUID, nodeUUID)
	return unitUUID
}
