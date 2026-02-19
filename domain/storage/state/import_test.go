// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// importSuite is a set of tests to assert the interface and contracts
// importing storage into this state package.
type importSuite struct {
	testing.ModelSuite
}

// TestImportSuite runs all of the tests contained in
// [importSuite].
func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	// Arrange
	ebsPoolUUID := s.newStoragePool(c, "ebs", "fspool").String()
	gcePoolUUID := s.newStoragePool(c, "gce", "testme").String()
	netNodeUUID := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	unit, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)
	args := []internal.ImportStorageInstanceArgs{
		{
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			Life:             int(life.Alive),
			StorageName:      "multi-fs",
			StorageKind:      "block",
			StorageID:        "multi-fs/0",
			PoolName:         "ebs",
			RequestedSizeMiB: uint64(1024),
			UnitName:         unitName,
		}, {
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			Life:             int(life.Alive),
			StorageName:      "another-fs",
			StorageKind:      "filesystem",
			StorageID:        "another-fs/2",
			PoolName:         "gce",
			RequestedSizeMiB: uint64(4048),
			UnitName:         unitName,
		}, { // Add a storage_instance without a unit name.
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			Life:             int(life.Alive),
			StorageName:      "test-fs",
			StorageKind:      "filesystem",
			StorageID:        "test-fs/9",
			PoolName:         "gce",
			RequestedSizeMiB: uint64(4048),
		},
	}

	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.getStorageInstances(c)
	c.Check(obtained, tc.SameContents, []importStorageInstance{
		{
			UUID:            args[0].UUID,
			CharmName:       "myapp",
			StorageName:     "multi-fs",
			StorageKindID:   0,
			StorageID:       "multi-fs/0",
			LifeID:          0,
			StoragePoolUUID: ebsPoolUUID,
			RequestedSize:   uint64(1024),
		}, {
			UUID:            args[1].UUID,
			CharmName:       "myapp",
			StorageName:     "another-fs",
			StorageKindID:   1,
			StorageID:       "another-fs/2",
			LifeID:          0,
			StoragePoolUUID: gcePoolUUID,
			RequestedSize:   uint64(4048),
		}, { // No unit name results in no charm name.
			UUID:            args[2].UUID,
			StorageName:     "test-fs",
			StorageKindID:   1,
			StorageID:       "test-fs/9",
			LifeID:          0,
			StoragePoolUUID: gcePoolUUID,
			RequestedSize:   uint64(4048),
		},
	})
	s.checkStorageUnitOwner(c, unit, 2)
}

func (s *importSuite) TestImportFilesystemsIAAS(c *tc.C) {
	// Arrange
	ebsPoolUUID := s.newStoragePool(c, "ebs", "fspool")
	gcePoolUUID := s.newStoragePool(c, "gce", "testme")

	ebsInstanceUUID := s.newStorageInstance(c, "ebs", "1", ebsPoolUUID)
	gceInstanceUUID := s.newStorageInstance(c, "gce", "1", gcePoolUUID)

	ebsFsUUID := tc.Must(c, storage.NewFilesystemUUID)
	gceFsUUID := tc.Must(c, storage.NewFilesystemUUID)
	azureFsUUID := tc.Must(c, storage.NewFilesystemUUID)

	args := []internal.ImportFilesystemIAASArgs{{
		UUID:                ebsFsUUID.String(),
		ID:                  "ebs-fs-1",
		SizeInMiB:           1024,
		ProviderID:          "provider-ebs-fs-1",
		StorageInstanceUUID: ebsInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storageprovisioning.ProvisionScopeMachine,
	}, {
		UUID:                gceFsUUID.String(),
		ID:                  "gce-fs-1",
		SizeInMiB:           2048,
		ProviderID:          "provider-gce-fs-1",
		StorageInstanceUUID: gceInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storageprovisioning.ProvisionScopeModel,
	}, {
		UUID:       azureFsUUID.String(),
		ID:         "azure-fs-1",
		SizeInMiB:  4096,
		ProviderID: "provider-azure-fs-1",
		// This filesystem is not attached to any storage instance
		StorageInstanceUUID: "",
		Life:                life.Alive,
		Scope:               storageprovisioning.ProvisionScopeModel,
	}}

	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportFilesystemsIAAS(c.Context(), args, nil)

	c.Assert(err, tc.ErrorIsNil)
	obtainedFs, obtainedFsInstances := s.getFilesystems(c)
	c.Check(obtainedFs, tc.SameContents, []importStorageFilesystem{{
		UUID:       ebsFsUUID.String(),
		ID:         "ebs-fs-1",
		LifeID:     int(life.Alive),
		ScopeID:    int(storageprovisioning.ProvisionScopeMachine),
		ProviderID: "provider-ebs-fs-1",
		SizeInMiB:  1024,
	}, {
		UUID:       gceFsUUID.String(),
		ID:         "gce-fs-1",
		LifeID:     int(life.Alive),
		ScopeID:    int(storageprovisioning.ProvisionScopeModel),
		ProviderID: "provider-gce-fs-1",
		SizeInMiB:  2048,
	}, {
		UUID:       azureFsUUID.String(),
		ID:         "azure-fs-1",
		LifeID:     int(life.Alive),
		ScopeID:    int(storageprovisioning.ProvisionScopeModel),
		ProviderID: "provider-azure-fs-1",
		SizeInMiB:  4096,
	}})
	c.Check(obtainedFsInstances, tc.SameContents, []importStorageInstanceFilesystem{{
		StorageInstanceUUID: ebsInstanceUUID.String(),
		FilesystemUUID:      ebsFsUUID.String(),
	}, {
		StorageInstanceUUID: gceInstanceUUID.String(),
		FilesystemUUID:      gceFsUUID.String(),
	}})
}

func (s *importSuite) TestImportFilesystemsIAASWithAttachments(c *tc.C) {
	// Arrange
	ebsPoolUUID := s.newStoragePool(c, "ebs", "fspool")
	gcePoolUUID := s.newStoragePool(c, "gce", "testme")

	ebsInstanceUUID := s.newStorageInstance(c, "ebs", "1", ebsPoolUUID)
	gceInstanceUUID := s.newStorageInstance(c, "gce", "1", gcePoolUUID)

	netNodeUUID1 := s.newNetNode(c)
	netNodeUUID2 := s.newNetNode(c)
	netNodeUUID3 := s.newNetNode(c)

	ebsFsUUID := tc.Must(c, storage.NewFilesystemUUID)
	gceFsUUID := tc.Must(c, storage.NewFilesystemUUID)

	ebsAttachment1UUID := tc.Must(c, storage.NewFilesystemAttachmentUUID)
	ebsAttachment2UUID := tc.Must(c, storage.NewFilesystemAttachmentUUID)
	gceAttachmentUUID := tc.Must(c, storage.NewFilesystemAttachmentUUID)

	fsArgs := []internal.ImportFilesystemIAASArgs{{
		UUID:                ebsFsUUID.String(),
		ID:                  "ebs-fs-1",
		SizeInMiB:           1024,
		ProviderID:          "provider-ebs-fs-1",
		StorageInstanceUUID: ebsInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storageprovisioning.ProvisionScopeMachine,
	}, {
		UUID:                gceFsUUID.String(),
		ID:                  "gce-fs-1",
		SizeInMiB:           2048,
		ProviderID:          "provider-gce-fs-1",
		StorageInstanceUUID: gceInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storageprovisioning.ProvisionScopeModel,
	}}

	attachmentArgs := []internal.ImportFilesystemAttachmentIAASArgs{{
		UUID:           ebsAttachment1UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		Scope:          storageprovisioning.ProvisionScopeMachine,
		NetNodeUUID:    netNodeUUID1,
		MountPoint:     "/mnt/ebs1",
		ReadOnly:       false,
	}, {
		UUID:           ebsAttachment2UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		Scope:          storageprovisioning.ProvisionScopeMachine,
		NetNodeUUID:    netNodeUUID2,
		MountPoint:     "/mnt/ebs2",
		ReadOnly:       true,
	}, {
		UUID:           gceAttachmentUUID.String(),
		FilesystemUUID: gceFsUUID.String(),
		Scope:          storageprovisioning.ProvisionScopeModel,
		NetNodeUUID:    netNodeUUID3,
		MountPoint:     "/mnt/gce",
		ReadOnly:       false,
	}}

	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportFilesystemsIAAS(c.Context(), fsArgs, attachmentArgs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedAttachments := s.getFilesystemAttachments(c)
	c.Check(obtainedAttachments, tc.SameContents, []importStorageFilesystemAttachment{{
		UUID:           ebsAttachment1UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		NetNodeUUID:    netNodeUUID1,
		ScopeID:        int(storageprovisioning.ProvisionScopeMachine),
		LifeID:         int(life.Alive),
		MountPoint:     "/mnt/ebs1",
		ReadOnly:       false,
	}, {
		UUID:           ebsAttachment2UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		NetNodeUUID:    netNodeUUID2,
		ScopeID:        int(storageprovisioning.ProvisionScopeMachine),
		LifeID:         int(life.Alive),
		MountPoint:     "/mnt/ebs2",
		ReadOnly:       true,
	}, {
		UUID:           gceAttachmentUUID.String(),
		FilesystemUUID: gceFsUUID.String(),
		NetNodeUUID:    netNodeUUID3,
		ScopeID:        int(storageprovisioning.ProvisionScopeModel),
		LifeID:         int(life.Alive),
		MountPoint:     "/mnt/gce",
		ReadOnly:       false,
	}})
}

func (s *importSuite) TestImportVolumes(c *tc.C) {
	// Arrange
	ebsPoolUUID := s.newStoragePool(c, "ebs", "fspool")

	ebsInstanceUUID := s.newStorageInstance(c, "ebs", "1", ebsPoolUUID).String()

	args := []internal.ImportVolumeArgs{
		{
			UUID:                tc.Must(c, storage.NewVolumeUUID).String(),
			ID:                  "0",
			ProviderID:          "vol-0f2829d7e5c4c0140",
			LifeID:              life.Alive,
			ProvisionScopeID:    storageprovisioning.ProvisionScopeMachine,
			WWN:                 "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
			Persistent:          true,
			SizeMiB:             1024,
			StorageInstanceUUID: ebsInstanceUUID,
		},
	}
	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportVolumes(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.getStorageVolumes(c)
	c.Check(obtained, tc.SameContents, []importStorageVolume{
		{
			UUID:             args[0].UUID,
			VolumeID:         args[0].ID,
			ProviderID:       args[0].ProviderID,
			LifeID:           int(args[0].LifeID),
			ProvisionScopeID: int(args[0].ProvisionScopeID),
			WWN:              args[0].WWN,
			Persistent:       true,
			SizeMiB:          args[0].SizeMiB,
		},
	})
	obtainedInstances := s.getStorageInstanceVolumes(c)
	c.Check(obtainedInstances, tc.SameContents, []importStorageInstanceVolume{
		{
			StorageInstanceUUID: ebsInstanceUUID,
			VolumeUUID:          args[0].UUID,
		},
	})
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitName(c *tc.C) {
	// Arrange 1 machine with a net node
	netNodeUUID2 := s.newNetNode(c)
	s.newMachine(c, "42", netNodeUUID2)

	// Arrange 1 unit with a net node
	netNodeUUID1 := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID1)

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []string{"42"}, []string{"foo/0"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.DeepEquals, map[string]string{
		"42": netNodeUUID2,
	})
	c.Check(obtainedUnits, tc.DeepEquals, map[string]string{
		"foo/0": netNodeUUID1,
	})
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitNameMachineNotFound(c *tc.C) {
	// Arrange 1 machine with a net node
	netNodeUUID2 := s.newNetNode(c)
	s.newMachine(c, "42", netNodeUUID2)

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []string{"42"}, []string{"fake/0"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.DeepEquals, map[string]string{
		"42": netNodeUUID2,
	})
	c.Check(obtainedUnits, tc.HasLen, 0)
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitNameUnitNotFound(c *tc.C) {
	// Arrange 1 unit with a net node
	netNodeUUID1 := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID1)

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []string{"42"}, []string{"foo/0"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.HasLen, 0)
	c.Check(obtainedUnits, tc.DeepEquals, map[string]string{
		"foo/0": netNodeUUID1,
	})
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitNameNoInput(c *tc.C) {
	// Arrange
	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []string{""}, []string{""})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.HasLen, 0)
	c.Check(obtainedUnits, tc.HasLen, 0)
}

func (s *importSuite) TestGetMachineUUIDByName(c *tc.C) {
	// Arrange
	st := NewState(s.TxnRunnerFactory())
	expectedUUID := s.newMachine(c, "42", s.newNetNode(c))

	// Act
	obtainedUUID, err := st.GetMachineUUIDByName(c.Context(), "42")

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedUUID, tc.Equals, expectedUUID)
}

func (s *importSuite) TestGetMachineUUIDByNameNotFound(c *tc.C) {
	// Arrange
	st := NewState(s.TxnRunnerFactory())

	// Act
	_, err := st.GetMachineUUIDByName(c.Context(), "42")

	// Assert
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *importSuite) TestGetMachineUUIDByNameDead(c *tc.C) {
	// Arrange
	st := NewState(s.TxnRunnerFactory())
	s.newMachineWithLife(c, "42", s.newNetNode(c), life.Dead)

	// Act
	_, err := st.GetMachineUUIDByName(c.Context(), "42")

	// Assert
	c.Assert(err, tc.ErrorIs, machineerrors.MachineIsDead)
}

func (s *importSuite) TestGetBlockDevicesForMachineMany(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	machineUUID := s.newMachine(c, "666", netNodeUUID)

	bd1 := coreblockdevice.BlockDevice{
		DeviceName:      "name-666",
		FilesystemLabel: "label-666",
		FilesystemUUID:  "device-666",
		HardwareId:      "hardware-666",
		WWN:             "wwn-666",
		BusAddress:      "bus-666",
		SizeMiB:         666,
		FilesystemType:  "btrfs",
		InUse:           true,
		MountPoint:      "mount-666",
		SerialId:        "serial-666",
	}
	bd2 := coreblockdevice.BlockDevice{
		DeviceName:      "name-667",
		DeviceLinks:     []string{"dev_link1", "dev_link2"},
		FilesystemLabel: "label-667",
		FilesystemUUID:  "device-667",
		HardwareId:      "hardware-667",
		WWN:             "wwn-667",
		BusAddress:      "bus-667",
		SizeMiB:         667,
		FilesystemType:  "btrfs",
		MountPoint:      "mount-667",
		SerialId:        "serial-667",
	}
	blockDevice1UUID := s.newBlockDevice(c, machineUUID, bd1)
	blockDevice2UUID := s.newBlockDevice(c, machineUUID, bd2)

	result, err := st.GetBlockDevicesForMachine(c.Context(), machine.UUID(machineUUID))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals,
		map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
			blockdevice.BlockDeviceUUID(blockDevice1UUID): bd1,
			blockdevice.BlockDeviceUUID(blockDevice2UUID): bd2,
		},
	)
}

func (s *importSuite) TestGetBlockDevicesForMachineOnMachine(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID1 := s.newNetNode(c)
	machine1UUID := s.newMachine(c, "666", netNodeUUID1)
	netNodeUUID2 := s.newNetNode(c)
	machine2UUID := s.newMachine(c, "667", netNodeUUID2)

	bd1 := coreblockdevice.BlockDevice{
		DeviceName:      "name-666",
		FilesystemLabel: "label-666",
		FilesystemUUID:  "device-666",
		HardwareId:      "hardware-666",
		WWN:             "wwn-666",
		BusAddress:      "bus-666",
		SizeMiB:         666,
		FilesystemType:  "btrfs",
		InUse:           true,
		MountPoint:      "mount-666",
		SerialId:        "serial-666",
	}
	bd2 := coreblockdevice.BlockDevice{
		DeviceName:      "name-667",
		DeviceLinks:     []string{"dev_link1", "dev_link2"},
		FilesystemLabel: "label-667",
		FilesystemUUID:  "device-667",
		HardwareId:      "hardware-667",
		WWN:             "wwn-667",
		BusAddress:      "bus-667",
		SizeMiB:         667,
		FilesystemType:  "btrfs",
		MountPoint:      "mount-667",
		SerialId:        "serial-667",
	}
	s.newBlockDevice(c, machine1UUID, bd1)
	blockDevice2UUID := s.newBlockDevice(c, machine2UUID, bd2)

	result, err := st.GetBlockDevicesForMachine(c.Context(), machine.UUID(machine2UUID))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals,
		map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
			blockdevice.BlockDeviceUUID(blockDevice2UUID): bd2,
		},
	)
}

func (s *importSuite) getStorageInstances(c *tc.C) []importStorageInstance {
	var result []importStorageInstance
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id, storage_pool_uuid, requested_size_mib 
FROM storage_instance`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uuid, charm_name, storageName, storageID, pool string
			var size uint64
			var kind, life int
			if err := rows.Scan(&uuid, &charm_name, &storageName, &kind, &storageID, &life, &pool, &size); err != nil {
				return err
			}
			result = append(result, importStorageInstance{
				UUID:            uuid,
				CharmName:       charm_name,
				StorageName:     storageName,
				StorageKindID:   kind,
				StoragePoolUUID: pool,
				StorageID:       storageID,
				LifeID:          life,
				RequestedSize:   size,
			})
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) getStorageVolumes(c *tc.C) []importStorageVolume {
	var result []importStorageVolume
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, volume_id, life_id, provision_scope_id, provider_id, size_mib, wwn, persistent 
FROM storage_volume`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uuid, volumeID, providerID, wwn, persistent string
			var sizeMIB, scope, life int
			if err := rows.Scan(&uuid, &volumeID, &life, &scope, &providerID, &sizeMIB, &wwn, &persistent); err != nil {
				return err
			}
			result = append(result, importStorageVolume{
				UUID:             uuid,
				VolumeID:         volumeID,
				ProviderID:       providerID,
				LifeID:           life,
				ProvisionScopeID: scope,
				WWN:              wwn,
				Persistent:       true,
				SizeMiB:          uint64(sizeMIB),
			})
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) getStorageInstanceVolumes(c *tc.C) []importStorageInstanceVolume {
	var result []importStorageInstanceVolume
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT storage_instance_uuid, storage_volume_uuid 
FROM storage_instance_volume`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var storageInstanceUUID, storageVolumeUUID string
			if err := rows.Scan(&storageInstanceUUID, &storageVolumeUUID); err != nil {
				return err
			}
			result = append(result, importStorageInstanceVolume{
				StorageInstanceUUID: storageInstanceUUID,
				VolumeUUID:          storageVolumeUUID,
			})
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) checkStorageUnitOwner(c *tc.C, unitUUID string, expected int) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_unit_owner WHERE unit_uuid = $1`, unitUUID).Scan(&count)
		if err != nil {
			return errors.Errorf("getting owner count: %w", err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected)
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *importSuite) newStoragePool(c *tc.C,
	name string, providerType string,
) storage.StoragePoolUUID {
	spUUID := storagetesting.GenStoragePoolUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, ?, ?)`, spUUID.String(), name, providerType)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return spUUID
}

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *importSuite) newNetNode(c *tc.C) string {
	nodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID).String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *importSuite) newApplication(c *tc.C, name string) (string, string) {
	appUUID := tc.Must(c, uuid.NewUUID)

	charmUUID := s.newCharm(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID, name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String(), charmUUID
}

// newCharm creates a new charm in the model and returns the uuid for it.
func (s *importSuite) newCharm(c *tc.C) string {
	charmUUID := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(
		c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
				charmUUID.String(), "foo",
			)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')
`,
				charmUUID.String(),
			)
			return err
		})
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID.String()
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *importSuite) newUnitWithNetNode(
	c *tc.C, unitName, appUUID, netNodeUUID string,
) (string, string) {
	var charmUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid FROM application WHERE uuid = ?",
		appUUID,
	).Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	unit := tc.Must(c, uuid.NewUUID).String()

	_, err = s.DB().ExecContext(
		c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`, unit, unitName, appUUID, charmUUID, netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return unit, unitName
}

func (s *importSuite) newMachineWithLife(c *tc.C, name, netNodeUUID string, life life.Life) string {
	machineUUID := tc.Must(c, uuid.NewUUID).String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO machine (uuid, net_node_uuid, name, life_id)
		VALUES (?, ?, ?, ?)`, machineUUID, netNodeUUID, name, life)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func (s *importSuite) newMachine(c *tc.C, name, netNodeUUID string) string {
	return s.newMachineWithLife(c, name, netNodeUUID, life.Alive)
}

func (s *importSuite) getFilesystems(c *tc.C) ([]importStorageFilesystem, []importStorageInstanceFilesystem) {
	var filesystems []importStorageFilesystem
	var instanceFilesystems []importStorageInstanceFilesystem
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		fsRows, err := tx.QueryContext(c.Context(), `
SELECT uuid, filesystem_id, life_id, provision_scope_id, provider_id, size_mib
FROM storage_filesystem`)
		if err != nil {
			return err
		}

		defer func() { _ = fsRows.Close() }()
		for fsRows.Next() {
			var uuid, id, providerID string
			var lifeID, scopeID int
			var size uint64
			if err := fsRows.Scan(&uuid, &id, &lifeID, &scopeID, &providerID, &size); err != nil {
				return err
			}
			filesystems = append(filesystems, importStorageFilesystem{
				UUID:       uuid,
				ID:         id,
				LifeID:     lifeID,
				ScopeID:    scopeID,
				ProviderID: providerID,
				SizeInMiB:  size,
			})
		}
		if err := fsRows.Err(); err != nil {
			return err
		}

		instFsRows, err := tx.QueryContext(c.Context(), `
SELECT storage_instance_uuid, storage_filesystem_uuid
FROM storage_instance_filesystem`)
		if err != nil {
			return err
		}

		defer func() { _ = instFsRows.Close() }()
		for instFsRows.Next() {
			var instanceUUID, fsUUID string
			if err := instFsRows.Scan(&instanceUUID, &fsUUID); err != nil {
				return err
			}
			instanceFilesystems = append(instanceFilesystems, importStorageInstanceFilesystem{
				StorageInstanceUUID: instanceUUID,
				FilesystemUUID:      fsUUID,
			})
		}
		if err := instFsRows.Err(); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return filesystems, instanceFilesystems
}

func (s *importSuite) getFilesystemAttachments(c *tc.C) []importStorageFilesystemAttachment {
	var attachments []importStorageFilesystemAttachment
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, storage_filesystem_uuid, net_node_uuid, provision_scope_id, life_id, mount_point, read_only
FROM storage_filesystem_attachment`)
		if err != nil {
			return err
		}

		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uuid, fsUUID, netNodeUUID, mountPoint string
			var scopeID, lifeID int
			var readOnly bool
			if err := rows.Scan(&uuid, &fsUUID, &netNodeUUID, &scopeID, &lifeID, &mountPoint, &readOnly); err != nil {
				return err
			}
			attachments = append(attachments, importStorageFilesystemAttachment{
				UUID:           uuid,
				FilesystemUUID: fsUUID,
				NetNodeUUID:    netNodeUUID,
				ScopeID:        scopeID,
				LifeID:         lifeID,
				MountPoint:     mountPoint,
				ReadOnly:       readOnly,
			})
		}

		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return attachments
}

func (s *importSuite) newStorageInstance(c *tc.C, name, id string, poolUUID storage.StoragePoolUUID) storage.StorageInstanceUUID {
	siUUID := tc.Must(c, storage.NewStorageInstanceUUID)

	fullID := fmt.Sprintf("%s/%s", name, id)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance (uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id, storage_pool_uuid, requested_size_mib)
VALUES (?, "foo", ?, 1, ?, 0, ?, 4048)`, siUUID, name, fullID, poolUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return siUUID
}

func (s *importSuite) newBlockDevice(c *tc.C, machineUUID string, bd coreblockdevice.BlockDevice) string {
	blockDeviceUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID).String()
	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := s.DB().Exec(`
INSERT INTO block_device (
	uuid, machine_uuid, name, filesystem_label,
	host_filesystem_uuid, hardware_id, wwn, bus_address, serial_id,
	mount_point, filesystem_type, size_mib, in_use)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, blockDeviceUUID, machineUUID, bd.DeviceName, bd.FilesystemLabel,
		bd.FilesystemUUID, bd.HardwareId, bd.WWN, bd.BusAddress, bd.SerialId,
		bd.MountPoint, bd.FilesystemType, bd.SizeMiB, inUse)
	c.Assert(err, tc.ErrorIsNil)

	for _, link := range bd.DeviceLinks {
		_, err = s.DB().Exec(`
INSERT INTO block_device_link_device (block_device_uuid, machine_uuid, name)
VALUES (?, ?, ?)
`, blockDeviceUUID, machineUUID, link)
		c.Assert(err, tc.ErrorIsNil)
	}
	return blockDeviceUUID
}
