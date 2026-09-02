// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/internal/uuid"
)

// ---------------------------------------------------------------------------
// Helpers for provisioned machine test setup
// ---------------------------------------------------------------------------

// addMachineCloudInstanceRow inserts the mandatory machine_cloud_instance row
// (with NULL instance_id) and a machine_cloud_instance_status row. This
// mirrors what domain/machine/state does when a machine is created.
func (s *modelStateSuite) addMachineCloudInstanceRow(c *tc.C, machineUUID string) {
	s.runQuery(c,
		`INSERT INTO machine_cloud_instance (machine_uuid, life_id) VALUES (?,0)`,
		machineUUID)
	s.runQuery(c,
		`INSERT INTO machine_cloud_instance_status (machine_uuid, status_id, updated_at) VALUES (?,1,datetime('now'))`,
		machineUUID)
}

// addAvailabilityZoneNamed inserts an AZ with a specific name and returns its
// UUID. Uses INSERT OR IGNORE so the test can be run multiple times safely.
func (s *modelStateSuite) addAvailabilityZoneNamed(c *tc.C, name string) string {
	azUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT OR IGNORE INTO availability_zone (uuid, name) VALUES (?,?)`, azUUID, name)
	var actual string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT uuid FROM availability_zone WHERE name = ?`, name).Scan(&actual)
	})
	c.Assert(err, tc.ErrorIsNil)
	return actual
}

// addStorageVolume inserts a minimal storage_volume row and returns its UUID.
func (s *modelStateSuite) addStorageVolume(c *tc.C, volumeID string) string {
	volUUID := uuid.MustNewUUID().String()
	// provision_scope_id=1 = machine, life_id=0 = alive
	s.runQuery(c,
		`INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?,?,0,1)`,
		volUUID, volumeID)
	return volUUID
}

// addStorageVolumeAttachment inserts a storage_volume_attachment for the given
// volume and net node. Returns the attachment UUID.
func (s *modelStateSuite) addStorageVolumeAttachment(c *tc.C, volumeUUID, netNodeUUID string) string {
	attUUID := uuid.MustNewUUID().String()
	s.runQuery(c,
		`INSERT INTO storage_volume_attachment (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id) VALUES (?,?,?,0,1)`,
		attUUID, volumeUUID, netNodeUUID)
	return attUUID
}

// addStorageVolumeAttachmentPlan inserts a storage_volume_attachment_plan for
// the given volume and net node. Returns the plan UUID.
func (s *modelStateSuite) addStorageVolumeAttachmentPlan(c *tc.C, volumeUUID, netNodeUUID string) string {
	planUUID := uuid.MustNewUUID().String()
	s.runQuery(c,
		`INSERT INTO storage_volume_attachment_plan (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id) VALUES (?,?,?,0,1)`,
		planUUID, volumeUUID, netNodeUUID)
	return planUUID
}

// queryCloudInstance reads back a machine_cloud_instance row.
func (s *modelStateSuite) queryCloudInstance(c *tc.C, machineUUID string) struct {
	InstanceID           string
	DisplayName          string
	Arch                 sql.NullString
	Mem                  sql.NullInt64
	RootDisk             sql.NullInt64
	VirtType             sql.NullString
	AvailabilityZoneUUID sql.NullString
} {
	var row struct {
		InstanceID           string
		DisplayName          string
		Arch                 sql.NullString
		Mem                  sql.NullInt64
		RootDisk             sql.NullInt64
		VirtType             sql.NullString
		AvailabilityZoneUUID sql.NullString
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT COALESCE(instance_id,''), COALESCE(display_name,''),
       arch, mem, root_disk, virt_type, availability_zone_uuid
FROM   machine_cloud_instance
WHERE  machine_uuid = ?`, machineUUID).Scan(
			&row.InstanceID, &row.DisplayName,
			&row.Arch, &row.Mem, &row.RootDisk, &row.VirtType,
			&row.AvailabilityZoneUUID,
		)
	})
	c.Assert(err, tc.ErrorIsNil)
	return row
}

// queryInstanceTags returns all tags for a machine.
func (s *modelStateSuite) queryInstanceTags(c *tc.C, machineUUID string) []string {
	var tags []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT tag FROM instance_tag WHERE machine_uuid = ? ORDER BY tag`, machineUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		localTags := make([]string, 0)
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				return err
			}
			localTags = append(localTags, t)
		}
		tags = localTags
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return tags
}

// queryLLDs returns link_layer_device rows for a machine.
func (s *modelStateSuite) queryLLDs(c *tc.C, netNodeUUID string) []struct {
	UUID string
	Name string
} {
	var devs []struct {
		UUID string
		Name string
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT uuid, name FROM link_layer_device WHERE net_node_uuid = ? ORDER BY name`,
			netNodeUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		localDevs := make([]struct {
			UUID string
			Name string
		}, 0)
		for rows.Next() {
			var d struct {
				UUID string
				Name string
			}
			if err := rows.Scan(&d.UUID, &d.Name); err != nil {
				return err
			}
			localDevs = append(localDevs, d)
		}
		devs = localDevs
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return devs
}

// queryIPAddresses returns ip_address rows for a net node, ordered by value.
func (s *modelStateSuite) queryIPAddresses(c *tc.C, netNodeUUID string) []struct {
	UUID        string
	Value       string
	OriginID    int
	SubnetUUID  sql.NullString
	DeviceUUID  string
	IsSecondary bool
	IsShadow    bool
} {
	type addrRow struct {
		UUID        string
		Value       string
		OriginID    int
		SubnetUUID  sql.NullString
		DeviceUUID  string
		IsSecondary bool
		IsShadow    bool
	}
	var rows []addrRow
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rs, err := tx.QueryContext(ctx, `
SELECT uuid, address_value, origin_id, subnet_uuid, device_uuid, is_secondary, is_shadow
FROM   ip_address
WHERE  net_node_uuid = ?
ORDER  BY address_value`, netNodeUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rs.Close() }()
		localRows := make([]addrRow, 0)
		for rs.Next() {
			var r addrRow
			if err := rs.Scan(&r.UUID, &r.Value, &r.OriginID, &r.SubnetUUID, &r.DeviceUUID, &r.IsSecondary, &r.IsShadow); err != nil {
				return err
			}
			localRows = append(localRows, r)
		}
		rows = localRows
		return rs.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	result := make([]struct {
		UUID        string
		Value       string
		OriginID    int
		SubnetUUID  sql.NullString
		DeviceUUID  string
		IsSecondary bool
		IsShadow    bool
	}, len(rows))
	for i, r := range rows {
		result[i].UUID = r.UUID
		result[i].Value = r.Value
		result[i].OriginID = r.OriginID
		result[i].SubnetUUID = r.SubnetUUID
		result[i].DeviceUUID = r.DeviceUUID
		result[i].IsSecondary = r.IsSecondary
		result[i].IsShadow = r.IsShadow
	}
	return result
}

// queryProviderDeviceIDs returns provider_link_layer_device rows for a net node.
func (s *modelStateSuite) queryProviderDeviceIDs(c *tc.C, netNodeUUID string) map[string]string {
	var result map[string]string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT plld.provider_id, plld.device_uuid
FROM   provider_link_layer_device plld
JOIN   link_layer_device lld ON lld.uuid = plld.device_uuid
WHERE  lld.net_node_uuid = ?`, netNodeUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		localResult := make(map[string]string)
		for rows.Next() {
			var pID, devUUID string
			if err := rows.Scan(&pID, &devUUID); err != nil {
				return err
			}
			localResult[devUUID] = pID
		}
		result = localResult
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

// queryProviderAddressIDs returns provider_ip_address provider_id keyed by
// address value for a given net node.
func (s *modelStateSuite) queryProviderAddressIDs(c *tc.C, netNodeUUID string) map[string]string {
	var result map[string]string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT ia.address_value, pia.provider_id
FROM   provider_ip_address pia
JOIN   ip_address ia ON ia.uuid = pia.address_uuid
WHERE  ia.net_node_uuid = ?`, netNodeUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		localResult := make(map[string]string)
		for rows.Next() {
			var addr, pID string
			if err := rows.Scan(&addr, &pID); err != nil {
				return err
			}
			localResult[addr] = pID
		}
		result = localResult
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

// queryVolumeProvisionedInfo reads provider_id and size_mib for a volume.
func (s *modelStateSuite) queryVolumeProvisionedInfo(c *tc.C, volumeUUID string) struct {
	ProviderID string
	SizeMiB    int64
	HardwareID string
	WWN        string
	Persistent bool
} {
	var row struct {
		ProviderID string
		SizeMiB    int64
		HardwareID string
		WWN        string
		Persistent bool
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COALESCE(provider_id,''), COALESCE(size_mib,0), COALESCE(hardware_id,''), COALESCE(wwn,''), COALESCE(persistent,0)
			 FROM storage_volume WHERE uuid = ?`, volumeUUID).
			Scan(&row.ProviderID, &row.SizeMiB, &row.HardwareID, &row.WWN, &row.Persistent)
	})
	c.Assert(err, tc.ErrorIsNil)
	return row
}

// queryVolumeAttachmentProvisionedInfo reads provisioned info for an attachment.
func (s *modelStateSuite) queryVolumeAttachmentProvisionedInfo(c *tc.C, attachmentUUID string) struct {
	ReadOnly        bool
	BlockDeviceUUID sql.NullString
} {
	var row struct {
		ReadOnly        bool
		BlockDeviceUUID sql.NullString
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COALESCE(read_only,0), block_device_uuid FROM storage_volume_attachment WHERE uuid = ?`,
			attachmentUUID).Scan(&row.ReadOnly, &row.BlockDeviceUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return row
}

// queryPlanProvisionedInfo reads device_type_id for an attachment plan.
func (s *modelStateSuite) queryPlanProvisionedInfo(c *tc.C, planUUID string) struct {
	DeviceTypeID int
	Attrs        map[string]string
} {
	var row struct {
		DeviceTypeID int
		Attrs        map[string]string
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var localRow struct {
			DeviceTypeID int
			Attrs        map[string]string
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(device_type_id,0) FROM storage_volume_attachment_plan WHERE uuid = ?`,
			planUUID).Scan(&localRow.DeviceTypeID); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT key, value FROM storage_volume_attachment_plan_attr WHERE attachment_plan_uuid = ?`,
			planUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		localRow.Attrs = make(map[string]string)
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			localRow.Attrs[k] = v
		}
		row = localRow
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return row
}

// getNetNodeForMachine returns the net_node_uuid for a machine.
func (s *modelStateSuite) getNetNodeForMachine(c *tc.C, machineUUID string) string {
	var nodeUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID).Scan(&nodeUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return nodeUUID
}

// ---------------------------------------------------------------------------
// SetMachineCloudInstance tests
// ---------------------------------------------------------------------------

// TestSetMachineCloudInstanceMinimal verifies a basic provisioning write with
// no hardware characteristics.
func (s *modelStateSuite) TestSetMachineCloudInstanceMinimal(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "0", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	err := s.state.SetMachineCloudInstance(
		c.Context(), machineUUID, "inst-1", "display-1", "nonce-1", nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	row := s.queryCloudInstance(c, machineUUID)
	c.Check(row.InstanceID, tc.Equals, "inst-1")
	c.Check(row.DisplayName, tc.Equals, "display-1")
	c.Check(row.Arch.Valid, tc.IsFalse)
}

// TestSetMachineCloudInstanceWithHardwareCharacteristics verifies hardware
// characteristics are persisted correctly.
func (s *modelStateSuite) TestSetMachineCloudInstanceWithHardwareCharacteristics(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "1", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	hw := &instance.HardwareCharacteristics{
		Arch:     new("amd64"),
		Mem:      new(uint64(4096)),
		RootDisk: new(uint64(20480)),
		CpuCores: new(uint64(4)),
		CpuPower: new(uint64(2000)),
		VirtType: new("hvm"),
	}

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID, "inst-hw", "hw-machine", "nonce-hw", hw)
	c.Assert(err, tc.ErrorIsNil)

	row := s.queryCloudInstance(c, machineUUID)
	c.Check(row.InstanceID, tc.Equals, "inst-hw")
	c.Assert(row.Arch.Valid, tc.IsTrue)
	c.Check(row.Arch.String, tc.Equals, "amd64")
	c.Assert(row.Mem.Valid, tc.IsTrue)
	c.Check(row.Mem.Int64, tc.Equals, int64(4096))
	c.Assert(row.RootDisk.Valid, tc.IsTrue)
	c.Check(row.RootDisk.Int64, tc.Equals, int64(20480))
	c.Assert(row.VirtType.Valid, tc.IsTrue)
	c.Check(row.VirtType.String, tc.Equals, "hvm")
}

// TestSetMachineCloudInstanceWithAvailabilityZone verifies that the AZ name is
// resolved to its UUID and stored on the machine cloud instance row.
func (s *modelStateSuite) TestSetMachineCloudInstanceWithAvailabilityZone(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "2", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	azUUID := s.addAvailabilityZoneNamed(c, "us-east-1a")
	hw := &instance.HardwareCharacteristics{AvailabilityZone: new("us-east-1a")}

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID, "inst-az", "az-machine", "nonce", hw)
	c.Assert(err, tc.ErrorIsNil)

	row := s.queryCloudInstance(c, machineUUID)
	c.Assert(row.AvailabilityZoneUUID.Valid, tc.IsTrue)
	c.Check(row.AvailabilityZoneUUID.String, tc.Equals, azUUID)
}

// TestSetMachineCloudInstanceWithInstanceTags verifies that hardware tags are
// inserted into instance_tag.
func (s *modelStateSuite) TestSetMachineCloudInstanceWithInstanceTags(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "3", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	tags := []string{"env=production", "team=platform"}
	hw := &instance.HardwareCharacteristics{Tags: &tags}

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID, "inst-tags", "tagged-machine", "nonce", hw)
	c.Assert(err, tc.ErrorIsNil)

	got := s.queryInstanceTags(c, machineUUID)
	c.Assert(got, tc.HasLen, 2)
	c.Check(got[0], tc.Equals, "env=production")
	c.Check(got[1], tc.Equals, "team=platform")
}

// TestSetMachineCloudInstanceAlreadyProvisioned verifies that calling the
// method a second time returns MachineCloudInstanceAlreadyExists without
// overwriting the existing data.
func (s *modelStateSuite) TestSetMachineCloudInstanceAlreadyProvisioned(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "4", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID, "inst-first", "first", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Second call should be rejected.
	err = s.state.SetMachineCloudInstance(c.Context(), machineUUID, "inst-second", "second", "nonce2", nil)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineCloudInstanceAlreadyExists)

	// Data must be from the first call.
	row := s.queryCloudInstance(c, machineUUID)
	c.Check(row.InstanceID, tc.Equals, "inst-first")
}

// TestSetMachineCloudInstanceManualPrefix verifies that an instance ID with
// the manual prefix causes a machine_manual row to be inserted.
func (s *modelStateSuite) TestSetMachineCloudInstanceManualPrefix(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "5", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID, "manual:my-host", "manual-machine", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	var count int
	errQ := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM machine_manual WHERE machine_uuid = ?`, machineUUID,
		).Scan(&count)
	})
	c.Assert(errQ, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestSetMachineCloudInstanceNonceSetOnlyWhenEmpty verifies that the nonce is
// only written when the machine row currently has no nonce.
func (s *modelStateSuite) TestSetMachineCloudInstanceNonceSetOnlyWhenEmpty(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "6", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	// Pre-set a nonce on the machine.
	s.runQuery(c, `UPDATE machine SET nonce = 'existing-nonce' WHERE uuid = ?`, machineUUID)

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID, "inst-nonce", "nonce-machine", "new-nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// The nonce should remain the pre-set value, not overwritten.
	var nonce sql.NullString
	errQ := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT nonce FROM machine WHERE uuid = ?`, machineUUID).Scan(&nonce)
	})
	c.Assert(errQ, tc.ErrorIsNil)
	c.Check(nonce.String, tc.Equals, "existing-nonce")
}

// ---------------------------------------------------------------------------
// RecordProvisionedMachineNetConfig tests
// ---------------------------------------------------------------------------

// TestRecordProvisionedMachineNetConfigNoNICs verifies that passing an empty
// NIC slice is a no-op.
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigNoNICs(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "10", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nil)
	c.Assert(err, tc.ErrorIsNil)

	devs := s.queryLLDs(c, netNodeUUID)
	c.Check(devs, tc.HasLen, 0)
}

// TestRecordProvisionedMachineNetConfigMachineNotFound verifies that a
// non-existent machine UUID returns MachineNotFound.
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigMachineNotFound(c *tc.C) {
	nics := []domainnetwork.NetInterface{{Name: "eth0"}}
	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), "no-such-uuid", nics)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestRecordProvisionedMachineNetConfigSingleNIC verifies devices and
// addresses are inserted correctly on a first write.
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigSingleNIC(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "11", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	nics := []domainnetwork.NetInterface{{
		Name:       "eth0",
		ProviderID: new(corenetwork.Id("provider-eth0")),
		Type:       corenetwork.EthernetDevice,
		IsEnabled:  true,
		Addrs: []domainnetwork.NetAddr{{
			InterfaceName: "eth0",
			AddressValue:  "10.0.0.5/24",
			AddressType:   corenetwork.IPv4Address,
			ConfigType:    corenetwork.ConfigDHCP,
			Origin:        corenetwork.OriginProvider,
			Scope:         corenetwork.ScopeCloudLocal,
			ProviderID:    new(corenetwork.Id("provider-addr-1")),
		}},
	}}

	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nics)
	c.Assert(err, tc.ErrorIsNil)

	// Verify device was inserted.
	devs := s.queryLLDs(c, netNodeUUID)
	c.Assert(devs, tc.HasLen, 1)
	c.Check(devs[0].Name, tc.Equals, "eth0")

	// Verify provider device ID.
	provDevIDs := s.queryProviderDeviceIDs(c, netNodeUUID)
	c.Assert(provDevIDs, tc.HasLen, 1)
	c.Check(provDevIDs[devs[0].UUID], tc.Equals, "provider-eth0")

	// Verify address was inserted.
	addrs := s.queryIPAddresses(c, netNodeUUID)
	c.Assert(addrs, tc.HasLen, 1)
	c.Check(addrs[0].Value, tc.Equals, "10.0.0.5/24")
	c.Check(addrs[0].OriginID, tc.Equals, 1) // provider

	// Verify provider address ID.
	provAddrIDs := s.queryProviderAddressIDs(c, netNodeUUID)
	c.Check(provAddrIDs["10.0.0.5/24"], tc.Equals, "provider-addr-1")
}

// TestRecordProvisionedMachineNetConfigShadowAddress verifies that shadow
// addresses (is_shadow=true) from the provider result are stored correctly.
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigShadowAddress(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "12", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	nics := []domainnetwork.NetInterface{{
		Name:      "eth0",
		Type:      corenetwork.EthernetDevice,
		IsEnabled: true,
		Addrs: []domainnetwork.NetAddr{
			{
				InterfaceName: "eth0",
				AddressValue:  "10.0.0.5/24",
				AddressType:   corenetwork.IPv4Address,
				ConfigType:    corenetwork.ConfigDHCP,
				Origin:        corenetwork.OriginProvider,
				Scope:         corenetwork.ScopeCloudLocal,
				IsShadow:      false,
			},
			{
				InterfaceName: "eth0",
				AddressValue:  "54.1.2.3/32",
				AddressType:   corenetwork.IPv4Address,
				ConfigType:    corenetwork.ConfigDHCP,
				Origin:        corenetwork.OriginProvider,
				Scope:         corenetwork.ScopePublic,
				IsShadow:      true,
			},
		},
	}}

	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nics)
	c.Assert(err, tc.ErrorIsNil)

	addrs := s.queryIPAddresses(c, netNodeUUID)
	c.Assert(addrs, tc.HasLen, 2)

	shadowCount := 0
	for _, a := range addrs {
		if a.IsShadow {
			shadowCount++
			c.Check(a.Value, tc.Equals, "54.1.2.3/32")
		}
	}
	c.Check(shadowCount, tc.Equals, 1)
}

// TestRecordProvisionedMachineNetConfigSubnetResolution verifies that
// when a provider subnet ID matches a known subnet, the address is linked
// to that subnet.
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigSubnetResolution(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "13", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	// Add a subnet and register a provider subnet ID for it.
	spaceUUID := s.addSpace(c, "default")
	subnetUUID := s.addSubnet(c, spaceUUID, "10.0.0.0/24")
	s.addProviderSubnet(c, subnetUUID, "psubnet-1")

	nics := []domainnetwork.NetInterface{{
		Name:      "eth0",
		Type:      corenetwork.EthernetDevice,
		IsEnabled: true,
		Addrs: []domainnetwork.NetAddr{{
			InterfaceName:    "eth0",
			AddressValue:     "10.0.0.7/24",
			AddressType:      corenetwork.IPv4Address,
			ConfigType:       corenetwork.ConfigDHCP,
			Origin:           corenetwork.OriginProvider,
			Scope:            corenetwork.ScopeCloudLocal,
			ProviderSubnetID: new(corenetwork.Id("psubnet-1")),
		}},
	}}

	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nics)
	c.Assert(err, tc.ErrorIsNil)

	addrs := s.queryIPAddresses(c, netNodeUUID)
	c.Assert(addrs, tc.HasLen, 1)
	c.Assert(addrs[0].SubnetUUID.Valid, tc.IsTrue)
	c.Check(addrs[0].SubnetUUID.String, tc.Equals, subnetUUID)
}

// TestRecordProvisionedMachineNetConfigIsIdempotent verifies that calling the
// method twice with the same data produces the same result (upsert semantics
// for retry safety).
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigIsIdempotent(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "14", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	nics := []domainnetwork.NetInterface{{
		Name:       "eth0",
		ProviderID: new(corenetwork.Id("provider-eth0")),
		Type:       corenetwork.EthernetDevice,
		IsEnabled:  true,
		Addrs: []domainnetwork.NetAddr{{
			InterfaceName: "eth0",
			AddressValue:  "10.0.0.9/24",
			AddressType:   corenetwork.IPv4Address,
			ConfigType:    corenetwork.ConfigDHCP,
			Origin:        corenetwork.OriginProvider,
			Scope:         corenetwork.ScopeCloudLocal,
		}},
	}}

	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nics)
	c.Assert(err, tc.ErrorIsNil)

	// Second call with same data — must not fail (ON CONFLICT DO UPDATE).
	err = s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nics)
	c.Assert(err, tc.ErrorIsNil)

	// Should still have exactly one device and one address.
	devs := s.queryLLDs(c, netNodeUUID)
	c.Check(devs, tc.HasLen, 1)
	addrs := s.queryIPAddresses(c, netNodeUUID)
	c.Check(addrs, tc.HasLen, 1)
}

// TestRecordProvisionedMachineNetConfigMultipleNICs verifies multiple NICs
// are all inserted in one call.
func (s *modelStateSuite) TestRecordProvisionedMachineNetConfigMultipleNICs(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "15", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	nics := []domainnetwork.NetInterface{
		{
			Name:      "eth0",
			Type:      corenetwork.EthernetDevice,
			IsEnabled: true,
			Addrs: []domainnetwork.NetAddr{{
				InterfaceName: "eth0",
				AddressValue:  "10.0.0.1/24",
				AddressType:   corenetwork.IPv4Address,
				ConfigType:    corenetwork.ConfigDHCP,
				Origin:        corenetwork.OriginProvider,
				Scope:         corenetwork.ScopeCloudLocal,
			}},
		},
		{
			Name:      "eth1",
			Type:      corenetwork.EthernetDevice,
			IsEnabled: true,
			Addrs: []domainnetwork.NetAddr{{
				InterfaceName: "eth1",
				AddressValue:  "192.168.0.1/16",
				AddressType:   corenetwork.IPv4Address,
				ConfigType:    corenetwork.ConfigDHCP,
				Origin:        corenetwork.OriginProvider,
				Scope:         corenetwork.ScopeCloudLocal,
			}},
		},
	}

	err := s.state.RecordProvisionedMachineNetConfig(c.Context(), machineUUID, nics)
	c.Assert(err, tc.ErrorIsNil)

	devs := s.queryLLDs(c, netNodeUUID)
	c.Assert(devs, tc.HasLen, 2)

	addrs := s.queryIPAddresses(c, netNodeUUID)
	c.Assert(addrs, tc.HasLen, 2)
}

// ---------------------------------------------------------------------------
// RecordProvisionedVolumes tests
// ---------------------------------------------------------------------------

// TestRecordProvisionedVolumesEmpty verifies that an empty slice is a no-op.
func (s *modelStateSuite) TestRecordProvisionedVolumesEmpty(c *tc.C) {
	err := s.state.RecordProvisionedVolumes(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

// TestRecordProvisionedVolumesSingle verifies that a single volume's provider
// info is written correctly.
func (s *modelStateSuite) TestRecordProvisionedVolumesSingle(c *tc.C) {
	volUUID := s.addStorageVolume(c, "vol/0")

	err := s.state.RecordProvisionedVolumes(c.Context(), []provisioner.ProvisionedVolume{{
		VolumeID:   "vol/0",
		ProviderID: "provider-vol-1",
		SizeMiB:    10240,
		HardwareID: "hw-123",
		WWN:        "wwn-abc",
		Persistent: true,
	}})
	c.Assert(err, tc.ErrorIsNil)

	info := s.queryVolumeProvisionedInfo(c, volUUID)
	c.Check(info.ProviderID, tc.Equals, "provider-vol-1")
	c.Check(info.SizeMiB, tc.Equals, int64(10240))
	c.Check(info.HardwareID, tc.Equals, "hw-123")
	c.Check(info.WWN, tc.Equals, "wwn-abc")
	c.Check(info.Persistent, tc.IsTrue)
}

// TestRecordProvisionedVolumesMultiple verifies multiple volumes are all
// written in the same call.
func (s *modelStateSuite) TestRecordProvisionedVolumesMultiple(c *tc.C) {
	vol1UUID := s.addStorageVolume(c, "vol/1")
	vol2UUID := s.addStorageVolume(c, "vol/2")

	err := s.state.RecordProvisionedVolumes(c.Context(), []provisioner.ProvisionedVolume{
		{VolumeID: "vol/1", ProviderID: "pv-1", SizeMiB: 1024},
		{VolumeID: "vol/2", ProviderID: "pv-2", SizeMiB: 2048},
	})
	c.Assert(err, tc.ErrorIsNil)

	info1 := s.queryVolumeProvisionedInfo(c, vol1UUID)
	c.Check(info1.ProviderID, tc.Equals, "pv-1")
	c.Check(info1.SizeMiB, tc.Equals, int64(1024))

	info2 := s.queryVolumeProvisionedInfo(c, vol2UUID)
	c.Check(info2.ProviderID, tc.Equals, "pv-2")
	c.Check(info2.SizeMiB, tc.Equals, int64(2048))
}

// TestRecordProvisionedVolumesVolumeNotFound verifies that a missing volume ID
// returns an error.
func (s *modelStateSuite) TestRecordProvisionedVolumesVolumeNotFound(c *tc.C) {
	err := s.state.RecordProvisionedVolumes(c.Context(), []provisioner.ProvisionedVolume{{
		VolumeID: "vol/nonexistent",
	}})
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorMatches, `.*vol/nonexistent.*`)
}

// TestRecordProvisionedVolumesIsIdempotent verifies that a second call
// overwrites the first (last-write-wins UPDATE semantics).
func (s *modelStateSuite) TestRecordProvisionedVolumesIsIdempotent(c *tc.C) {
	volUUID := s.addStorageVolume(c, "vol/idem")

	err := s.state.RecordProvisionedVolumes(c.Context(), []provisioner.ProvisionedVolume{{
		VolumeID: "vol/idem", ProviderID: "pv-first", SizeMiB: 100,
	}})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.RecordProvisionedVolumes(c.Context(), []provisioner.ProvisionedVolume{{
		VolumeID: "vol/idem", ProviderID: "pv-second", SizeMiB: 200,
	}})
	c.Assert(err, tc.ErrorIsNil)

	info := s.queryVolumeProvisionedInfo(c, volUUID)
	c.Check(info.ProviderID, tc.Equals, "pv-second")
	c.Check(info.SizeMiB, tc.Equals, int64(200))
}

// ---------------------------------------------------------------------------
// RecordProvisionedVolumeAttachments tests
// ---------------------------------------------------------------------------

// TestRecordProvisionedVolumeAttachmentsEmpty verifies an empty map is a
// no-op.
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsEmpty(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "20", "ubuntu", "22.04/stable")
	err := s.state.RecordProvisionedVolumeAttachments(
		c.Context(), machineUUID, nil,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// TestRecordProvisionedVolumeAttachmentsMachineNotFound verifies that a
// missing machine UUID returns MachineNotFound.
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsMachineNotFound(c *tc.C) {
	err := s.state.RecordProvisionedVolumeAttachments(
		c.Context(), "no-such-machine",
		map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/0": {ReadOnly: true},
		},
	)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestRecordProvisionedVolumeAttachmentsReadOnly verifies that read_only is
// written and no block device is created when there is no device info.
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsReadOnly(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "21", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/ro")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)

	err := s.state.RecordProvisionedVolumeAttachments(
		c.Context(), machineUUID,
		map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/ro": {ReadOnly: true},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	info := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Check(info.ReadOnly, tc.IsTrue)
	c.Check(info.BlockDeviceUUID.Valid, tc.IsFalse)
}

// TestRecordProvisionedVolumeAttachmentsCreatesBlockDevice verifies that a new
// block device is created when device info is present and no matching device
// exists.
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsCreatesBlockDevice(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "22", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/bd")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)

	err := s.state.RecordProvisionedVolumeAttachments(
		c.Context(), machineUUID,
		map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/bd": {DeviceName: "sdb", BusAddress: "0:0:1:0"},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	info := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Assert(info.BlockDeviceUUID.Valid, tc.IsTrue)
	c.Check(info.BlockDeviceUUID.String, tc.Not(tc.Equals), "")

	// The block_device row should exist.
	var bdName string
	errQ := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COALESCE(name,'') FROM block_device WHERE uuid = ?`,
			info.BlockDeviceUUID.String).Scan(&bdName)
	})
	c.Assert(errQ, tc.ErrorIsNil)
	c.Check(bdName, tc.Equals, "sdb")
}

// TestRecordProvisionedVolumeAttachmentsMatchesExistingBlockDevice verifies
// that when a block device with the same name already exists on the machine,
// it is reused rather than creating a duplicate.
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsMatchesExistingBlockDevice(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "23", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	// Pre-insert a block device with name "sdc".
	existingBDUUID := uuid.MustNewUUID().String()
	s.runQuery(c,
		`INSERT INTO block_device (uuid, machine_uuid, name) VALUES (?,?,?)`,
		existingBDUUID, machineUUID, "sdc")

	volUUID := s.addStorageVolume(c, "vol/match")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)

	err := s.state.RecordProvisionedVolumeAttachments(
		c.Context(), machineUUID,
		map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/match": {DeviceName: "sdc"},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	info := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Assert(info.BlockDeviceUUID.Valid, tc.IsTrue)
	// Must reuse the existing block device, not create a new one.
	c.Check(info.BlockDeviceUUID.String, tc.Equals, existingBDUUID)
}

// TestRecordProvisionedVolumeAttachmentsWithPlan verifies that an attachment
// plan's device type and attributes are written correctly.
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsWithPlan(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "24", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/plan")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)
	planUUID := s.addStorageVolumeAttachmentPlan(c, volUUID, netNodeUUID)

	err := s.state.RecordProvisionedVolumeAttachments(
		c.Context(), machineUUID,
		map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/plan": {
				ReadOnly: false,
				Plan: &provisioner.ProvisionedVolumeAttachmentPlan{
					DeviceType: "iscsi",
					DeviceAttributes: map[string]string{
						"target": "iqn.2001-04.example:storage1",
						"lun":    "0",
					},
				},
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Attachment updated (no block device).
	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Check(attInfo.BlockDeviceUUID.Valid, tc.IsFalse)

	// Plan updated.
	planInfo := s.queryPlanProvisionedInfo(c, planUUID)
	c.Check(planInfo.DeviceTypeID, tc.Equals, 1) // iscsi = 1
	c.Assert(planInfo.Attrs, tc.HasLen, 2)
	c.Check(planInfo.Attrs["target"], tc.Equals, "iqn.2001-04.example:storage1")
	c.Check(planInfo.Attrs["lun"], tc.Equals, "0")
}

// TestRecordProvisionedVolumeAttachmentsIsIdempotent verifies the method can
// be called twice safely (retry scenario).
func (s *modelStateSuite) TestRecordProvisionedVolumeAttachmentsIsIdempotent(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "25", "ubuntu", "22.04/stable")
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/idem2")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)
	planUUID := s.addStorageVolumeAttachmentPlan(c, volUUID, netNodeUUID)

	attachments := map[string]provisioner.ProvisionedVolumeAttachment{
		"vol/idem2": {
			ReadOnly:   true,
			DeviceName: "sdd",
			Plan: &provisioner.ProvisionedVolumeAttachmentPlan{
				DeviceType:       "iscsi",
				DeviceAttributes: map[string]string{"key": "val"},
			},
		},
	}

	err := s.state.RecordProvisionedVolumeAttachments(c.Context(), machineUUID, attachments)
	c.Assert(err, tc.ErrorIsNil)

	// Second call — should succeed without errors.
	err = s.state.RecordProvisionedVolumeAttachments(c.Context(), machineUUID, attachments)
	c.Assert(err, tc.ErrorIsNil)

	// Verify state is consistent after two calls.
	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Check(attInfo.ReadOnly, tc.IsTrue)
	c.Assert(attInfo.BlockDeviceUUID.Valid, tc.IsTrue)

	planInfo := s.queryPlanProvisionedInfo(c, planUUID)
	c.Check(planInfo.DeviceTypeID, tc.Equals, 1)
	c.Assert(planInfo.Attrs, tc.HasLen, 1)
	c.Check(planInfo.Attrs["key"], tc.Equals, "val")

	// Exactly one block device should exist (not two).
	var bdCount int
	errQ := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM block_device WHERE machine_uuid = ?`, machineUUID,
		).Scan(&bdCount)
	})
	c.Assert(errQ, tc.ErrorIsNil)
	c.Check(bdCount, tc.Equals, 1)
}
