// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	machineerrors "github.com/juju/juju/domain/machine/errors"
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
	InterfaceTypeID int
	Attrs           map[string]string
} {
	var row struct {
		InterfaceTypeID int
		Attrs           map[string]string
	}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var localRow struct {
			InterfaceTypeID int
			Attrs           map[string]string
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(device_type_id,0) FROM storage_volume_attachment_plan WHERE uuid = ?`,
			planUUID).Scan(&localRow.InterfaceTypeID); err != nil {
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
// RecordProvisionedMachine tests
// ---------------------------------------------------------------------------

// --- Cloud instance ---

func (s *modelStateSuite) TestRecordProvisionedMachineMinimal(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "0", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "display-1",
		Nonce:       "nonce-1",
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	row := s.queryCloudInstance(c, machineUUID)
	c.Check(row.InstanceID, tc.Equals, "inst-1")
	c.Check(row.DisplayName, tc.Equals, "display-1")
	c.Check(row.Arch.Valid, tc.IsFalse)
}

func (s *modelStateSuite) TestRecordProvisionedMachineWithHardwareCharacteristics(c *tc.C) {
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
	info := provisioner.ProvisionedMachineInfo{
		InstanceID:              "inst-hw",
		DisplayName:             "hw-machine",
		Nonce:                   "nonce-hw",
		HardwareCharacteristics: hw,
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
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

func (s *modelStateSuite) TestRecordProvisionedMachineWithAvailabilityZone(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "2", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	azUUID := s.addAvailabilityZoneNamed(c, "us-east-1a")
	hw := &instance.HardwareCharacteristics{AvailabilityZone: new("us-east-1a")}

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:              "inst-az",
		DisplayName:             "az-machine",
		Nonce:                   "nonce",
		HardwareCharacteristics: hw,
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	row := s.queryCloudInstance(c, machineUUID)
	c.Assert(row.AvailabilityZoneUUID.Valid, tc.IsTrue)
	c.Check(row.AvailabilityZoneUUID.String, tc.Equals, azUUID)
}

func (s *modelStateSuite) TestRecordProvisionedMachineWithInstanceTags(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "3", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	tags := []string{"env=production", "team=platform"}
	hw := &instance.HardwareCharacteristics{Tags: &tags}

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:              "inst-tags",
		DisplayName:             "tagged-machine",
		Nonce:                   "nonce",
		HardwareCharacteristics: hw,
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	got := s.queryInstanceTags(c, machineUUID)
	c.Assert(got, tc.HasLen, 2)
	c.Check(got[0], tc.Equals, "env=production")
	c.Check(got[1], tc.Equals, "team=platform")
}

func (s *modelStateSuite) TestRecordProvisionedMachineAlreadyProvisioned(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "4", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-first",
		DisplayName: "first",
		Nonce:       "nonce",
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	info2 := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-second",
		DisplayName: "second",
		Nonce:       "nonce2",
	}
	err = s.state.RecordProvisionedMachine(c.Context(), machineUUID, info2)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineCloudInstanceAlreadyExists)

	row := s.queryCloudInstance(c, machineUUID)
	c.Check(row.InstanceID, tc.Equals, "inst-first")
}

func (s *modelStateSuite) TestRecordProvisionedMachineManualPrefix(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "5", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "manual:my-host",
		DisplayName: "manual-machine",
		Nonce:       "nonce",
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
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

func (s *modelStateSuite) TestRecordProvisionedMachineNonceSetOnlyWhenEmpty(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "6", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	s.runQuery(c, `UPDATE machine SET nonce = 'existing-nonce' WHERE uuid = ?`, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-nonce",
		DisplayName: "nonce-machine",
		Nonce:       "new-nonce",
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	var nonce sql.NullString
	errQ := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT nonce FROM machine WHERE uuid = ?`, machineUUID).Scan(&nonce)
	})
	c.Assert(errQ, tc.ErrorIsNil)
	c.Check(nonce.String, tc.Equals, "existing-nonce")
}

// --- Volumes ---

func (s *modelStateSuite) TestRecordProvisionedMachineVolumesEmpty(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "20", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) TestRecordProvisionedMachineVolumesSingle(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "21", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	volUUID := s.addStorageVolume(c, "vol/0")

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		Volumes: []provisioner.ProvisionedVolume{{
			VolumeID:   "vol/0",
			ProviderID: "provider-vol-1",
			SizeMiB:    10240,
			HardwareID: "hw-123",
			WWN:        "wwn-abc",
			Persistent: true,
		}},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	volInfo := s.queryVolumeProvisionedInfo(c, volUUID)
	c.Check(volInfo.ProviderID, tc.Equals, "provider-vol-1")
	c.Check(volInfo.SizeMiB, tc.Equals, int64(10240))
	c.Check(volInfo.HardwareID, tc.Equals, "hw-123")
	c.Check(volInfo.WWN, tc.Equals, "wwn-abc")
	c.Check(volInfo.Persistent, tc.IsTrue)
}

func (s *modelStateSuite) TestRecordProvisionedMachineVolumesMultiple(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "22", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	vol1UUID := s.addStorageVolume(c, "vol/1")
	vol2UUID := s.addStorageVolume(c, "vol/2")

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		Volumes: []provisioner.ProvisionedVolume{
			{VolumeID: "vol/1", ProviderID: "pv-1", SizeMiB: 1024},
			{VolumeID: "vol/2", ProviderID: "pv-2", SizeMiB: 2048},
		},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	info1 := s.queryVolumeProvisionedInfo(c, vol1UUID)
	c.Check(info1.ProviderID, tc.Equals, "pv-1")
	c.Check(info1.SizeMiB, tc.Equals, int64(1024))

	info2 := s.queryVolumeProvisionedInfo(c, vol2UUID)
	c.Check(info2.ProviderID, tc.Equals, "pv-2")
	c.Check(info2.SizeMiB, tc.Equals, int64(2048))
}

func (s *modelStateSuite) TestRecordProvisionedMachineVolumesNotFound(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "23", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		Volumes: []provisioner.ProvisionedVolume{{
			VolumeID: "vol/nonexistent",
		}},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorMatches, `.*vol/nonexistent.*`)
}

// --- Volume attachments ---

func (s *modelStateSuite) TestRecordProvisionedMachineAttachmentsEmpty(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "30", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) TestRecordProvisionedMachineAttachmentsMachineNotFound(c *tc.C) {
	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		VolumeAttachments: map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/0": {ReadOnly: true},
		},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), "no-such-machine", info)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *modelStateSuite) TestRecordProvisionedMachineAttachmentsReadOnly(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "31", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/ro")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		VolumeAttachments: map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/ro": {ReadOnly: true},
		},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Check(attInfo.ReadOnly, tc.IsTrue)
	c.Check(attInfo.BlockDeviceUUID.Valid, tc.IsFalse)
}

func (s *modelStateSuite) TestRecordProvisionedMachineAttachmentsCreatesBlockDevice(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "32", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/bd")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		VolumeAttachments: map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/bd": {DeviceName: "sdb", BusAddress: "0:0:1:0"},
		},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Assert(attInfo.BlockDeviceUUID.Valid, tc.IsTrue)
	c.Check(attInfo.BlockDeviceUUID.String, tc.Not(tc.Equals), "")

	var bdName string
	errQ := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COALESCE(name,'') FROM block_device WHERE uuid = ?`,
			attInfo.BlockDeviceUUID.String).Scan(&bdName)
	})
	c.Assert(errQ, tc.ErrorIsNil)
	c.Check(bdName, tc.Equals, "sdb")
}

func (s *modelStateSuite) TestRecordProvisionedMachineAttachmentsMatchesExistingBlockDevice(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "33", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	existingBDUUID := uuid.MustNewUUID().String()
	s.runQuery(c,
		`INSERT INTO block_device (uuid, machine_uuid, name) VALUES (?,?,?)`,
		existingBDUUID, machineUUID, "sdc")

	volUUID := s.addStorageVolume(c, "vol/match")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		VolumeAttachments: map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/match": {DeviceName: "sdc"},
		},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Assert(attInfo.BlockDeviceUUID.Valid, tc.IsTrue)
	c.Check(attInfo.BlockDeviceUUID.String, tc.Equals, existingBDUUID)
}

func (s *modelStateSuite) TestRecordProvisionedMachineAttachmentsWithPlan(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "34", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/plan")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)
	planUUID := s.addStorageVolumeAttachmentPlan(c, volUUID, netNodeUUID)

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:  "inst-1",
		DisplayName: "machine-1",
		Nonce:       "nonce",
		VolumeAttachments: map[string]provisioner.ProvisionedVolumeAttachment{
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
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Check(attInfo.BlockDeviceUUID.Valid, tc.IsFalse)

	planInfo := s.queryPlanProvisionedInfo(c, planUUID)
	c.Check(planInfo.InterfaceTypeID, tc.Equals, 1)
	c.Assert(planInfo.Attrs, tc.HasLen, 2)
	c.Check(planInfo.Attrs["target"], tc.Equals, "iqn.2001-04.example:storage1")
	c.Check(planInfo.Attrs["lun"], tc.Equals, "0")
}

// --- Combined (all two sub-operations) ---

func (s *modelStateSuite) TestRecordProvisionedMachineCombined(c *tc.C) {
	machineUUID := s.addMachineWithPlatform(c, "40", "ubuntu", "22.04/stable")
	s.addMachineCloudInstanceRow(c, machineUUID)
	netNodeUUID := s.getNetNodeForMachine(c, machineUUID)

	volUUID := s.addStorageVolume(c, "vol/0")
	attUUID := s.addStorageVolumeAttachment(c, volUUID, netNodeUUID)
	planUUID := s.addStorageVolumeAttachmentPlan(c, volUUID, netNodeUUID)

	hw := &instance.HardwareCharacteristics{
		Arch:     new("amd64"),
		Mem:      new(uint64(8192)),
		RootDisk: new(uint64(40960)),
	}

	info := provisioner.ProvisionedMachineInfo{
		InstanceID:              "inst-combined",
		DisplayName:             "combined",
		Nonce:                   "nonce",
		HardwareCharacteristics: hw,
		Volumes: []provisioner.ProvisionedVolume{{
			VolumeID:   "vol/0",
			ProviderID: "provider-vol-1",
			SizeMiB:    10240,
			Persistent: true,
		}},
		VolumeAttachments: map[string]provisioner.ProvisionedVolumeAttachment{
			"vol/0": {
				ReadOnly:   true,
				DeviceName: "sdb",
				Plan: &provisioner.ProvisionedVolumeAttachmentPlan{
					DeviceType:       "iscsi",
					DeviceAttributes: map[string]string{"key": "val"},
				},
			},
		},
	}
	err := s.state.RecordProvisionedMachine(c.Context(), machineUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	// Cloud instance.
	ci := s.queryCloudInstance(c, machineUUID)
	c.Check(ci.InstanceID, tc.Equals, "inst-combined")

	// Volumes.
	volInfo := s.queryVolumeProvisionedInfo(c, volUUID)
	c.Check(volInfo.ProviderID, tc.Equals, "provider-vol-1")

	// Attachments.
	attInfo := s.queryVolumeAttachmentProvisionedInfo(c, attUUID)
	c.Check(attInfo.ReadOnly, tc.IsTrue)
	c.Assert(attInfo.BlockDeviceUUID.Valid, tc.IsTrue)

	planInfo := s.queryPlanProvisionedInfo(c, planUUID)
	c.Check(planInfo.InterfaceTypeID, tc.Equals, 1)
	c.Check(planInfo.Attrs["key"], tc.Equals, "val")
}
