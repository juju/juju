// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
)

type linkLayerSuite struct {
	linkLayerBaseSuite
}

func TestLinkLayerSuite(t *testing.T) {
	tc.Run(t, &linkLayerSuite{})
}

func (s *linkLayerSuite) TestMachineInterfaceViewFitsType(c *tc.C) {
	db, err := s.TxnRunnerFactory()()
	c.Assert(err, tc.ErrorIsNil)

	// Arrange
	nodeUUID := "net-node-uuid"
	machineUUID := "machine-uuid"
	machineName := "0"
	devUUID := "dev-uuid"
	devName := "eth0"
	subUUID := "sub-uuid"
	addrUUID := "addr-uuid"

	ctx := c.Context()

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
			machineUUID, nodeUUID, machineName, 0,
		); err != nil {
			return err
		}

		insertLLD := `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES (?, ?, ?, ?, ?, ?, ?)`

		if _, err = tx.ExecContext(ctx, insertLLD, devUUID, nodeUUID, devName, 1500, "00:11:22:33:44:55", 0, 0); err != nil {
			return err
		}

		if _, err = tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)",
			subUUID, "10.0.0.0/24", corenetwork.AlphaSpaceId,
		); err != nil {
			return err
		}

		insertIPAddress := `
INSERT INTO ip_address (uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid, net_node_uuid) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

		_, err = tx.ExecContext(ctx, insertIPAddress, addrUUID, devUUID, "10.0.0.1", 0, 0, 0, 0, subUUID, nodeUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	stmt, err := sqlair.Prepare("SELECT &machineInterfaceRow.* FROM v_machine_interface", machineInterfaceRow{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	var rows []machineInterfaceRow
	err = db.Txn(ctx, func(ctx context.Context, txn *sqlair.TX) error {
		return txn.Query(ctx, stmt).GetAll(&rows)
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(rows, tc.HasLen, 1)

	r := rows[0]
	c.Check(r.MachineUUID, tc.Equals, machineUUID)
	c.Check(r.MachineName, tc.Equals, machineName)
	c.Check(r.DeviceUUID, tc.Equals, devUUID)
	c.Check(r.DeviceName, tc.Equals, devName)
	c.Check(r.AddressUUID.String, tc.Equals, addrUUID)
	c.Check(r.SubnetUUID.String, tc.Equals, subUUID)
}

func (s *linkLayerSuite) TestGetMachineNetNodeUUID(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	machineUUID := "machine-uuid"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	q := "INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, ?)"
	_, err = db.ExecContext(ctx, q, machineUUID, "666", nodeUUID, 0)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	gotUUID, err := s.state.GetMachineNetNodeUUID(ctx, machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, nodeUUID)
}

func (s *linkLayerSuite) TestGetMachineNetNodeUUIDNotFoundError(c *tc.C) {
	_, err := s.state.GetMachineNetNodeUUID(c.Context(), "machine-uuid")
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *linkLayerSuite) TestSetMachineNetConfig(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	devName := "eth0"
	subnetUUID := "subnet-uuid"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)",
		subnetUUID, "192.168.0.0/24", corenetwork.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.state.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{{
		Name:            devName,
		Type:            corenetwork.EthernetDevice,
		VirtualPortType: corenetwork.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
		Addrs: []network.NetAddr{{
			InterfaceName: devName,
			AddressValue:  "192.168.0.50/24",
			AddressType:   corenetwork.IPv4Address,
			ConfigType:    corenetwork.ConfigDHCP,
			Origin:        corenetwork.OriginMachine,
			Scope:         corenetwork.ScopeCloudLocal,
		}},
		DNSSearchDomains: []string{"search.maas.net"},
		DNSAddresses:     []string{"8.8.8.8"},
	}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	checkScalarResult(c, db, "SELECT name FROM link_layer_device", "eth0")
	checkScalarResult(c, db, "SELECT address_value FROM ip_address", "192.168.0.50/24")
	checkScalarResult(c, db, "SELECT subnet_uuid FROM ip_address", subnetUUID)
	checkScalarResult(c, db, "SELECT search_domain FROM link_layer_device_dns_domain", "search.maas.net")
	checkScalarResult(c, db, "SELECT dns_address FROM link_layer_device_dns_address", "8.8.8.8")
}

func (s *linkLayerSuite) TestSetMachineNetConfigMultipleSubnetMatch(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	devName := "eth0"
	subnetUUID1 := "subnet-uuid-1"
	subnetUUID2 := "subnet-uuid-2"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	for _, subnetUUID := range []string{subnetUUID1, subnetUUID2} {
		_, err = db.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)",
			subnetUUID, "192.168.0.0/24", corenetwork.AlphaSpaceId)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Act
	err = s.state.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{{
		Name:            devName,
		Type:            corenetwork.EthernetDevice,
		VirtualPortType: corenetwork.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
		Addrs: []network.NetAddr{{
			InterfaceName: devName,
			AddressValue:  "192.168.0.50/24",
			AddressType:   corenetwork.IPv4Address,
			ConfigType:    corenetwork.ConfigDHCP,
			Origin:        corenetwork.OriginMachine,
			Scope:         corenetwork.ScopeCloudLocal,
		}},
		DNSSearchDomains: []string{"search.maas.net"},
		DNSAddresses:     []string{"8.8.8.8"},
	}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	checkScalarResult(c, db, "SELECT name FROM link_layer_device", "eth0")
	checkScalarResult(c, db, "SELECT address_value FROM ip_address", "192.168.0.50/24")
	checkScalarResult(c, db, "SELECT search_domain FROM link_layer_device_dns_domain", "search.maas.net")
	checkScalarResult(c, db, "SELECT dns_address FROM link_layer_device_dns_address", "8.8.8.8")

	// Check that we created a new subnet and linked it to the address.
	row := db.QueryRowContext(ctx, "SELECT uuid, cidr FROM subnet WHERE uuid NOT IN (?, ?)", subnetUUID1, subnetUUID2)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var newSubUUID, cidr string
	err = row.Scan(&newSubUUID, &cidr)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cidr, tc.Equals, "192.168.0.50/32")

	checkScalarResult(c, db, "SELECT subnet_uuid FROM ip_address", newSubUUID)
}

func (s *linkLayerSuite) TestSetMachineNetConfigNoAddresses(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	devName := "eth0"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.state.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{{
		Name:            devName,
		Type:            corenetwork.EthernetDevice,
		VirtualPortType: corenetwork.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
	}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	checkScalarResult(c, db, "SELECT name FROM link_layer_device", "eth0")

	row := db.QueryRowContext(ctx, "SELECT count(*) FROM ip_address")
	c.Assert(row.Err(), tc.ErrorIsNil)

	var addrCount int
	err = row.Scan(&addrCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrCount, tc.Equals, 0)
}

func (s *linkLayerSuite) TestSetMachineNetConfigUpdatedNIC(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	devName := "eth0"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act: insert then update.
	nic := network.NetInterface{
		Name:            devName,
		Type:            corenetwork.EthernetDevice,
		VirtualPortType: corenetwork.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
	}

	err = s.state.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{nic})
	c.Assert(err, tc.ErrorIsNil)

	nic.VLANTag = uint64(30)
	err = s.state.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{nic})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	checkScalarResult(c, db, "SELECT vlan_tag FROM link_layer_device", "30")
}

func (s *linkLayerSuite) TestSetMachineNetConfigWithParentDevices(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	devName := "eth0"
	brName := "br0"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.state.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{
		{
			Name:             devName,
			Type:             corenetwork.EthernetDevice,
			VirtualPortType:  corenetwork.NonVirtualPort,
			IsAutoStart:      true,
			IsEnabled:        true,
			ParentDeviceName: brName,
		},
		{
			Name:            brName,
			Type:            corenetwork.BridgeDevice,
			VirtualPortType: corenetwork.NonVirtualPort,
			IsAutoStart:     true,
			IsEnabled:       true,
		},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	parentSQL := `
SELECT dp.name 
FROM   link_layer_device AS dp
	   JOIN link_layer_device_parent AS p ON dp.uuid = p.parent_uuid
	   JOIN link_layer_device AS dc ON p.device_uuid = dc.uuid	
WHERE  dc.name = 'eth0'`

	checkScalarResult(c, db, parentSQL, brName)
}

func checkScalarResult(c *tc.C, db *sql.DB, query string, expected string) {
	rows, err := db.QueryContext(c.Context(), query)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		actual   string
		rowCount int
	)

	for rows.Next() {
		err = rows.Scan(&actual)
		c.Assert(err, tc.ErrorIsNil)
		rowCount++
	}

	c.Assert(rowCount, tc.Equals, 1)
	c.Check(actual, tc.Equals, expected)
}
