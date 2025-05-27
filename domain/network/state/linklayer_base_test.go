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
	"github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type linkLayerSuite struct {
	schematesting.ModelSuite
}

func (s *linkLayerSuite) SetupTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
}

func TestLinkLayerSuite(t *testing.T) {
	tc.Run(t, &linkLayerSuite{})
}

func (s *linkLayerSuite) TestMachineInterfaceViewFitsType(c *tc.C) {
	db, err := s.TxnRunnerFactory()()
	c.Assert(err, tc.ErrorIsNil)

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

	stmt, err := sqlair.Prepare("SELECT &machineInterfaceRow.* FROM v_machine_interface", machineInterfaceRow{})
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

// TODO (manadart 2025-05-26) this test is temporary.
// Future changes will reconcile existing devices and update them.
func (s *linkLayerSuite) TestSetMachineNetConfigAlreadySet(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	nodeUUID := "net-node-uuid"
	devName := "eth0"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	insertLLD := `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) 
VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = db.ExecContext(ctx, insertLLD, "dev-uuid", nodeUUID, devName, 1500, "00:11:22:33:44:55", 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{{Name: "eth1"}})
	c.Assert(err, tc.ErrorIsNil)

	rows, err := db.QueryContext(ctx, "SELECT name FROM link_layer_device")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		name  string
		count int
	)

	for rows.Next() {
		err = rows.Scan(&name)
		c.Assert(err, tc.ErrorIsNil)
		count++
	}

	c.Assert(count, tc.Equals, 1)
	// Incoming device "eth1" was ignored.
	c.Check(name, tc.Equals, "eth0")
}

func (s *linkLayerSuite) TestSetMachineNetConfig(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	nodeUUID := "net-node-uuid"
	devName := "eth0"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{{
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
	}})
	c.Assert(err, tc.ErrorIsNil)

	rows, err := db.QueryContext(ctx, "SELECT name FROM link_layer_device")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		name     string
		devCount int
	)

	for rows.Next() {
		err = rows.Scan(&name)
		c.Assert(err, tc.ErrorIsNil)
		devCount++
	}

	c.Assert(devCount, tc.Equals, 1)
	c.Check(name, tc.Equals, "eth0")

	rows, err = db.QueryContext(ctx, "SELECT address_value FROM ip_address")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		ip        string
		addrCount int
	)

	for rows.Next() {
		err = rows.Scan(&ip)
		c.Assert(err, tc.ErrorIsNil)
		addrCount++
	}

	c.Assert(addrCount, tc.Equals, 1)
	c.Check(ip, tc.Equals, "192.168.0.50/24")
}
func (s *linkLayerSuite) TestSetMachineNetConfigNoAddresses(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	db := s.DB()

	nodeUUID := "net-node-uuid"
	devName := "eth0"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetMachineNetConfig(ctx, "net-node-uuid", []network.NetInterface{{
		Name:            devName,
		Type:            corenetwork.EthernetDevice,
		VirtualPortType: corenetwork.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
	}})
	c.Assert(err, tc.ErrorIsNil)

	rows, err := db.QueryContext(ctx, "SELECT name FROM link_layer_device")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		name     string
		devCount int
	)

	for rows.Next() {
		err = rows.Scan(&name)
		c.Assert(err, tc.ErrorIsNil)
		devCount++
	}

	c.Assert(devCount, tc.Equals, 1)
	c.Check(name, tc.Equals, "eth0")

	row := db.QueryRowContext(ctx, "SELECT count(*) FROM ip_address")
	c.Assert(row.Err(), tc.ErrorIsNil)

	var addrCount int
	err = row.Scan(&addrCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrCount, tc.Equals, 0)
}
