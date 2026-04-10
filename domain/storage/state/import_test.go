// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreblockdevice "github.com/juju/juju/core/blockdevice"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
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
	appUUID, _ := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnit(c, "foo/0", appUUID)
	args := []internal.ImportStorageInstanceArgs{
		{
			UUID:              tc.Must(c, storage.NewStorageInstanceUUID).String(),
			Life:              life.Alive,
			StorageName:       "multi-fs",
			StorageKind:       "block",
			StorageInstanceID: "multi-fs/0",
			PoolName:          "ebs",
			RequestedSizeMiB:  uint64(1024),
			UnitUUID:          unitUUID.String(),
		}, {
			UUID:              tc.Must(c, storage.NewStorageInstanceUUID).String(),
			Life:              life.Dying,
			StorageName:       "another-fs",
			StorageKind:       "filesystem",
			StorageInstanceID: "another-fs/2",
			PoolName:          "gce",
			RequestedSizeMiB:  uint64(4048),
			UnitUUID:          unitUUID.String(),
		}, { // Add a storage_instance without a unit uuid.
			UUID:              tc.Must(c, storage.NewStorageInstanceUUID).String(),
			Life:              life.Dead,
			StorageName:       "test-fs",
			StorageKind:       "filesystem",
			StorageInstanceID: "test-fs/9",
			PoolName:          "gce",
			RequestedSizeMiB:  uint64(4048),
		},
	}

	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportStorageInstances(c.Context(), args, nil)

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
			LifeID:          1,
			StoragePoolUUID: gcePoolUUID,
			RequestedSize:   uint64(4048),
		}, { // No unit name results in no charm name.
			UUID:            args[2].UUID,
			StorageName:     "test-fs",
			StorageKindID:   1,
			StorageID:       "test-fs/9",
			LifeID:          2,
			StoragePoolUUID: gcePoolUUID,
			RequestedSize:   uint64(4048),
		},
	})
	s.checkStorageUnitOwner(c, unitUUID, 2)
}

func (s *importSuite) TestImportStorageInstancesWithAttachments(c *tc.C) {
	// Arrange
	s.newStoragePool(c, "ebs", "fspool")
	s.newStoragePool(c, "gce", "testme")
	appUUID, _ := s.newApplication(c, "foo")

	unit0UUID, _ := s.newUnit(c, "foo/0", appUUID)
	unit1UUID, _ := s.newUnit(c, "foo/1", appUUID)

	instance0UUID := tc.Must(c, storage.NewStorageInstanceUUID)
	instance1UUID := tc.Must(c, storage.NewStorageInstanceUUID)

	instanceArgs := []internal.ImportStorageInstanceArgs{
		{
			UUID:              instance0UUID.String(),
			Life:              life.Alive,
			StorageName:       "multi-fs",
			StorageKind:       "block",
			StorageInstanceID: "multi-fs/0",
			PoolName:          "ebs",
			RequestedSizeMiB:  uint64(1024),
			UnitUUID:          unit0UUID.String(),
		}, {
			UUID:              instance1UUID.String(),
			Life:              life.Alive,
			StorageName:       "another-fs",
			StorageKind:       "filesystem",
			StorageInstanceID: "another-fs/2",
			PoolName:          "gce",
			RequestedSizeMiB:  uint64(4048),
			UnitUUID:          unit0UUID.String(),
		},
	}
	attachmentArgs := []internal.ImportStorageInstanceAttachmentArgs{
		{
			UUID:                tc.Must(c, storage.NewStorageAttachmentUUID).String(),
			StorageInstanceUUID: instance0UUID.String(),
			UnitUUID:            unit0UUID.String(),
			Life:                life.Alive,
		},
		{
			UUID:                tc.Must(c, storage.NewStorageAttachmentUUID).String(),
			StorageInstanceUUID: instance0UUID.String(),
			UnitUUID:            unit1UUID.String(),
			Life:                life.Alive,
		},
		{
			UUID:                tc.Must(c, storage.NewStorageAttachmentUUID).String(),
			StorageInstanceUUID: instance1UUID.String(),
			UnitUUID:            unit1UUID.String(),
			Life:                life.Alive,
		},
	}

	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportStorageInstances(c.Context(), instanceArgs, attachmentArgs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.getStorageAttachments(c)
	c.Check(obtained, tc.SameContents, []importStorageAttachment{
		{
			UUID:                attachmentArgs[0].UUID,
			StorageInstanceUUID: instance0UUID.String(),
			UnitUUID:            unit0UUID.String(),
			LifeID:              int(life.Alive),
		},
		{
			UUID:                attachmentArgs[1].UUID,
			StorageInstanceUUID: instance0UUID.String(),
			UnitUUID:            unit1UUID.String(),
			LifeID:              int(life.Alive),
		},
		{
			UUID:                attachmentArgs[2].UUID,
			StorageInstanceUUID: instance1UUID.String(),
			UnitUUID:            unit1UUID.String(),
			LifeID:              int(life.Alive),
		},
	})
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

	args := []internal.ImportFilesystemArgs{{
		UUID:                ebsFsUUID.String(),
		ID:                  "ebs-fs-1",
		SizeInMiB:           1024,
		ProviderID:          "provider-ebs-fs-1",
		StorageInstanceUUID: ebsInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storage.ProvisionScopeMachine,
	}, {
		UUID:                gceFsUUID.String(),
		ID:                  "gce-fs-1",
		SizeInMiB:           2048,
		ProviderID:          "provider-gce-fs-1",
		StorageInstanceUUID: gceInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storage.ProvisionScopeModel,
	}, {
		UUID:       azureFsUUID.String(),
		ID:         "azure-fs-1",
		SizeInMiB:  4096,
		ProviderID: "provider-azure-fs-1",
		// This filesystem is not attached to any storage instance
		StorageInstanceUUID: "",
		Life:                life.Alive,
		Scope:               storage.ProvisionScopeModel,
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
		ScopeID:    int(storage.ProvisionScopeMachine),
		ProviderID: "provider-ebs-fs-1",
		SizeInMiB:  1024,
	}, {
		UUID:       gceFsUUID.String(),
		ID:         "gce-fs-1",
		LifeID:     int(life.Alive),
		ScopeID:    int(storage.ProvisionScopeModel),
		ProviderID: "provider-gce-fs-1",
		SizeInMiB:  2048,
	}, {
		UUID:       azureFsUUID.String(),
		ID:         "azure-fs-1",
		LifeID:     int(life.Alive),
		ScopeID:    int(storage.ProvisionScopeModel),
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

	fsArgs := []internal.ImportFilesystemArgs{{
		UUID:                ebsFsUUID.String(),
		ID:                  "ebs-fs-1",
		SizeInMiB:           1024,
		ProviderID:          "provider-ebs-fs-1",
		StorageInstanceUUID: ebsInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storage.ProvisionScopeMachine,
	}, {
		UUID:                gceFsUUID.String(),
		ID:                  "gce-fs-1",
		SizeInMiB:           2048,
		ProviderID:          "provider-gce-fs-1",
		StorageInstanceUUID: gceInstanceUUID.String(),
		Life:                life.Alive,
		Scope:               storage.ProvisionScopeModel,
	}}

	attachmentArgs := []internal.ImportFilesystemAttachmentArgs{{
		UUID:           ebsAttachment1UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		Scope:          storage.ProvisionScopeMachine,
		NetNodeUUID:    netNodeUUID1.String(),
		MountPoint:     "/mnt/ebs1",
		ProviderID:     "provider-id",
		ReadOnly:       false,
	}, {
		UUID:           ebsAttachment2UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		Scope:          storage.ProvisionScopeMachine,
		NetNodeUUID:    netNodeUUID2.String(),
		MountPoint:     "/mnt/ebs2",
		ReadOnly:       true,
	}, {
		UUID:           gceAttachmentUUID.String(),
		FilesystemUUID: gceFsUUID.String(),
		Scope:          storage.ProvisionScopeModel,
		NetNodeUUID:    netNodeUUID3.String(),
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
		NetNodeUUID:    netNodeUUID1.String(),
		ScopeID:        int(storage.ProvisionScopeMachine),
		LifeID:         int(life.Alive),
		MountPoint:     "/mnt/ebs1",
		ProviderID:     "provider-id",
		ReadOnly:       false,
	}, {
		UUID:           ebsAttachment2UUID.String(),
		FilesystemUUID: ebsFsUUID.String(),
		NetNodeUUID:    netNodeUUID2.String(),
		ScopeID:        int(storage.ProvisionScopeMachine),
		LifeID:         int(life.Alive),
		MountPoint:     "/mnt/ebs2",
		ReadOnly:       true,
	}, {
		UUID:           gceAttachmentUUID.String(),
		FilesystemUUID: gceFsUUID.String(),
		NetNodeUUID:    netNodeUUID3.String(),
		ScopeID:        int(storage.ProvisionScopeModel),
		LifeID:         int(life.Alive),
		MountPoint:     "/mnt/gce",
		ReadOnly:       false,
	}})
}

func (s *importSuite) TestImportVolumesFoundBlockDevice(c *tc.C) {
	// Arrange: the block device to be used.
	netNodeUUID := s.newNetNode(c)
	machineUUID := s.newMachine(c, "666", netNodeUUID.String())

	bd1 := coreblockdevice.BlockDevice{
		DeviceName: "name-666",
	}
	blockDeviceUUID := s.newBlockDevice(c, machineUUID, bd1)

	// Arrange: the storage instance to be used.
	ebsPoolUUID := s.newStoragePool(c, "ebs", "fspool")
	ebsInstanceUUID := s.newStorageInstance(c, "ebs", "1", ebsPoolUUID)

	// Arrange: input data with existing block device and storage instance.
	attachment := internal.ImportVolumeAttachmentArgs{
		UUID:            tc.Must(c, storage.NewVolumeAttachmentUUID),
		BlockDeviceUUID: blockDeviceUUID,
		LifeID:          life.Alive,
		NetNodeUUID:     netNodeUUID,
		ReadOnly:        false,
	}
	attachmentPlan := internal.ImportVolumeAttachmentPlanArgs{
		UUID:             tc.Must(c, storage.NewVolumeAttachmentPlanUUID),
		LifeID:           life.Alive,
		ProvisionScopeID: storage.ProvisionScopeMachine,
		DeviceAttributes: map[string]string{"foo": "bar", "baz": "food"},
		NetNodeUUID:      netNodeUUID,
	}
	args := []internal.ImportVolumeArgs{
		{
			UUID:                tc.Must(c, storage.NewVolumeUUID),
			ID:                  "0",
			ProviderID:          "vol-0f2829d7e5c4c0140",
			LifeID:              life.Alive,
			ProvisionScopeID:    storage.ProvisionScopeMachine,
			WWN:                 "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
			Persistent:          true,
			SizeMiB:             1024,
			StorageInstanceUUID: ebsInstanceUUID,
			Attachments:         []internal.ImportVolumeAttachmentArgs{attachment},
			AttachmentPlans:     []internal.ImportVolumeAttachmentPlanArgs{attachmentPlan},
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
			UUID:             args[0].UUID.String(),
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
			StorageInstanceUUID: ebsInstanceUUID.String(),
			VolumeUUID:          args[0].UUID.String(),
		},
	})
	obtainedAttachments := s.getStorageVolumeAttachments(c)
	c.Check(obtainedAttachments, tc.SameContents, []importStorageVolumeAttachment{
		{
			UUID:              attachment.UUID.String(),
			BlockDeviceUUID:   attachment.BlockDeviceUUID.String(),
			LifeID:            int(attachment.LifeID),
			NetNodeUUID:       netNodeUUID.String(),
			ReadOnly:          attachment.ReadOnly,
			ProvisionScopeID:  int(args[0].ProvisionScopeID),
			ProviderID:        "vol-0f2829d7e5c4c0140",
			StorageVolumeUUID: args[0].UUID.String(),
		},
	})
	obtainedAttachmentPlans := s.getStorageInstanceVolumeAttachmentPlans(c)
	c.Check(obtainedAttachmentPlans, tc.SameContents, []importStorageVolumeAttachmentPlan{
		{
			UUID:              attachmentPlan.UUID.String(),
			LifeID:            int(attachment.LifeID),
			NetNodeUUID:       netNodeUUID.String(),
			ProvisionScopeID:  int(args[0].ProvisionScopeID),
			StorageVolumeUUID: args[0].UUID.String(),
		},
	})
	obtainedAttachmentPlanAttrs := s.getStorageInstanceVolumeAttachmentPlanAttrs(c)
	c.Check(obtainedAttachmentPlanAttrs, tc.SameContents, []importStorageVolumePlanAttribute{
		{
			PlanUUID: attachmentPlan.UUID.String(),
			Key:      "foo",
			Value:    "bar",
		}, {
			PlanUUID: attachmentPlan.UUID.String(),
			Key:      "baz",
			Value:    "food",
		},
	})
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitName(c *tc.C) {
	// Arrange 1 machine with a net node
	netNodeUUID2 := s.newNetNode(c)
	s.newMachine(c, "42", netNodeUUID2.String())

	// Arrange 1 unit with a net node
	netNodeUUID1 := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID1.String())

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []machine.Name{"42"}, []coreunit.Name{"foo/0"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.DeepEquals, map[machine.Name]domainnetwork.NetNodeUUID{
		"42": netNodeUUID2,
	})
	c.Check(obtainedUnits, tc.DeepEquals, map[coreunit.Name]domainnetwork.NetNodeUUID{
		"foo/0": netNodeUUID1,
	})
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitNameMachineNotFound(c *tc.C) {
	// Arrange 1 machine with a net node
	netNodeUUID2 := s.newNetNode(c)
	s.newMachine(c, "42", netNodeUUID2.String())

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []machine.Name{"42"}, []coreunit.Name{"fake/0"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.DeepEquals, map[machine.Name]domainnetwork.NetNodeUUID{
		"42": netNodeUUID2,
	})
	c.Check(obtainedUnits, tc.HasLen, 0)
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitNameUnitNotFound(c *tc.C) {
	// Arrange 1 unit with a net node
	netNodeUUID1 := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID1.String())

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []machine.Name{"42"}, []coreunit.Name{"foo/0"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.HasLen, 0)
	c.Check(obtainedUnits, tc.DeepEquals, map[coreunit.Name]domainnetwork.NetNodeUUID{
		"foo/0": netNodeUUID1,
	})
}

func (s *importSuite) TestGetNetNodeUUIDsByMachineOrUnitNameNoInput(c *tc.C) {
	// Arrange
	st := NewState(s.TxnRunnerFactory())

	// Act
	obtainedMachines, obtainedUnits, err := st.GetNetNodeUUIDsByMachineOrUnitName(c.Context(), []machine.Name{""}, []coreunit.Name{""})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachines, tc.HasLen, 0)
	c.Check(obtainedUnits, tc.HasLen, 0)
}

func (s *importSuite) TestGetUnitUUIDsByNames(c *tc.C) {
	appUUID, _ := s.newApplication(c, "foo")
	unit0UUID, unit0Name := s.newUnit(c, "foo/0", appUUID)
	unit1UUID, unit1Name := s.newUnit(c, "foo/1", appUUID)
	unit2UUID, unit2Name := s.newUnit(c, "foo/2", appUUID)
	names := []string{unit0Name.String(), unit1Name.String(), unit2Name.String()}

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtained, err := st.GetUnitUUIDsByNames(c.Context(), names)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, map[string]string{
		unit0Name.String(): unit0UUID.String(),
		unit1Name.String(): unit1UUID.String(),
		unit2Name.String(): unit2UUID.String(),
	})
}

func (s *importSuite) TestGetUnitUUIDsByNamesNotFound(c *tc.C) {
	// Arrange
	appUUID, _ := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnit(c, "foo/0", appUUID)

	st := NewState(s.TxnRunnerFactory())

	// Act
	obtained, err := st.GetUnitUUIDsByNames(c.Context(), []string{"foo/0", "foo/1"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, map[string]string{
		"foo/0": unitUUID.String(),
	})
}

func (s *importSuite) TestGetUnitUUIDsByNamesNoInput(c *tc.C) {
	// Arrange
	st := NewState(s.TxnRunnerFactory())

	// Act
	obtained, err := st.GetUnitUUIDsByNames(c.Context(), []string{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.HasLen, 0)
}

func (s *importSuite) TestGetBlockDevicesForMachinesByNetNodeUUIDsMany(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	machineUUID := s.newMachine(c, "666", netNodeUUID.String())

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

	result, err := st.GetBlockDevicesForMachinesByNetNodeUUIDs(c.Context(),
		[]domainnetwork.NetNodeUUID{
			netNodeUUID,
		})
	c.Assert(err, tc.ErrorIsNil)
	devices, ok := result[netNodeUUID]
	c.Assert(ok, tc.Equals, true)
	c.Check(devices, tc.SameContents,
		[]internal.BlockDevice{
			{
				UUID:        blockDevice1UUID,
				BlockDevice: bd1,
			}, {
				UUID:        blockDevice2UUID,
				BlockDevice: bd2,
			},
		},
	)
}

func (s *importSuite) getStorageInstances(c *tc.C) []importStorageInstance {
	var result []importStorageInstance
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id, storage_pool_uuid, requested_size_mib 
FROM   storage_instance`)
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
FROM   storage_volume`)
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

func (s *importSuite) getStorageAttachments(c *tc.C) []importStorageAttachment {
	var result []importStorageAttachment
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, storage_instance_uuid, unit_uuid, life_id 
FROM storage_attachment`)
		if err != nil {
			return err
		}

		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uuid, instanceUUID, unitUUID string
			var life int
			if err := rows.Scan(&uuid, &instanceUUID, &unitUUID, &life); err != nil {
				return err
			}
			result = append(result, importStorageAttachment{
				UUID:                uuid,
				StorageInstanceUUID: instanceUUID,
				UnitUUID:            unitUUID,
				LifeID:              life,
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
FROM   storage_instance_volume`)
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
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) getStorageInstanceVolumeAttachmentPlans(c *tc.C) []importStorageVolumeAttachmentPlan {
	var result []importStorageVolumeAttachmentPlan
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id, device_type_id
FROM   storage_volume_attachment_plan`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				planUUID, storageVolumeUUID, netNodeUUID string
				lifeID, scopeID                          int
				deviceID                                 sql.NullInt64
			)
			if err := rows.Scan(&planUUID, &storageVolumeUUID, &netNodeUUID, &lifeID, &scopeID, &deviceID); err != nil {
				return err
			}
			result = append(result, importStorageVolumeAttachmentPlan{
				UUID:              planUUID,
				StorageVolumeUUID: storageVolumeUUID,
				NetNodeUUID:       netNodeUUID,
				LifeID:            lifeID,
				ProvisionScopeID:  scopeID,
				DeviceTypeID:      deviceID,
			})
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) getStorageVolumeAttachments(c *tc.C) []importStorageVolumeAttachment {
	var result []importStorageVolumeAttachment
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, block_device_uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id, provider_id, read_only
FROM   storage_volume_attachment`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				attachmentUUID, blockDeviceUUID, storageVolumeUUID, netNodeUUID, providerID string
				lifeID, scopeID                                                             int
				readOnly                                                                    bool
			)
			if err := rows.Scan(&attachmentUUID, &blockDeviceUUID, &storageVolumeUUID, &netNodeUUID, &lifeID, &scopeID, &providerID, &readOnly); err != nil {
				return err
			}
			result = append(result, importStorageVolumeAttachment{
				UUID:              attachmentUUID,
				BlockDeviceUUID:   blockDeviceUUID,
				StorageVolumeUUID: storageVolumeUUID,
				NetNodeUUID:       netNodeUUID,
				LifeID:            lifeID,
				ProvisionScopeID:  scopeID,
				ProviderID:        providerID,
				ReadOnly:          readOnly,
			})
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) getStorageInstanceVolumeAttachmentPlanAttrs(c *tc.C) []importStorageVolumePlanAttribute {
	var result []importStorageVolumePlanAttribute
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT attachment_plan_uuid, key, value 
FROM   storage_volume_attachment_plan_attr`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var attachmentPlanUUID, key, value string
			if err := rows.Scan(&attachmentPlanUUID, &key, &value); err != nil {
				return err
			}
			result = append(result, importStorageVolumePlanAttribute{
				PlanUUID: attachmentPlanUUID,
				Key:      key,
				Value:    value,
			})
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) checkStorageUnitOwner(c *tc.C, unitUUID coreunit.UUID, expected int) {
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
	spUUID := tc.Must(c, storage.NewStoragePoolUUID)

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
func (s *importSuite) newNetNode(c *tc.C) domainnetwork.NetNodeUUID {
	nodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *importSuite) newApplication(c *tc.C, name string) (string, string) {
	appUUID := tc.Must(c, coreapplication.NewUUID)

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

func (s *importSuite) newUnit(c *tc.C, unitName, appUUID string,
) (coreunit.UUID, coreunit.Name) {
	return s.newUnitWithNetNode(c, unitName, appUUID, s.newNetNode(c).String())
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *importSuite) newUnitWithNetNode(
	c *tc.C, unitName, appUUID, netNodeUUID string,
) (coreunit.UUID, coreunit.Name) {
	return s.newUnitWithNetNodeWithLife(c, unitName, appUUID, netNodeUUID, life.Alive)
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *importSuite) newUnitWithNetNodeWithLife(
	c *tc.C, unitName, appUUID, netNodeUUID string, life life.Life,
) (coreunit.UUID, coreunit.Name) {
	var charmUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid FROM application WHERE uuid = ?",
		appUUID,
	).Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	unit := tc.Must(c, coreunit.NewUUID)

	_, err = s.DB().ExecContext(
		c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, ?)
`, unit, unitName, appUUID, charmUUID, netNodeUUID, life,
	)
	c.Assert(err, tc.ErrorIsNil)

	return unit, coreunit.Name(unitName)
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
SELECT uuid, storage_filesystem_uuid, net_node_uuid, provision_scope_id, life_id, mount_point, provider_id, read_only
FROM storage_filesystem_attachment`)
		if err != nil {
			return err
		}

		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uuid, fsUUID, netNodeUUID, mountPoint, providerID string
			var scopeID, lifeID int
			var readOnly bool
			if err := rows.Scan(&uuid, &fsUUID, &netNodeUUID, &scopeID, &lifeID, &mountPoint, &providerID, &readOnly); err != nil {
				return err
			}
			attachments = append(attachments, importStorageFilesystemAttachment{
				UUID:           uuid,
				FilesystemUUID: fsUUID,
				NetNodeUUID:    netNodeUUID,
				ScopeID:        scopeID,
				LifeID:         lifeID,
				MountPoint:     mountPoint,
				ProviderID:     providerID,
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

func (s *importSuite) newBlockDevice(c *tc.C, machineUUID string, bd coreblockdevice.BlockDevice) blockdevice.BlockDeviceUUID {
	blockDeviceUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)
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
