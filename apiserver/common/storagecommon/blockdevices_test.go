// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/state"
)

type BlockDeviceSuite struct {
}

func TestBlockDeviceSuite(t *stdtesting.T) {
	tc.Run(t, &BlockDeviceSuite{})
}

func (s *BlockDeviceSuite) TestBlockDeviceMatchingSerialID(c *tc.C) {
	blockDevices := []blockdevice.BlockDevice{{
		DeviceName: "sdb",
		SerialId:   "543554ff-3b88-4",
	},
		{
			DeviceName: "sdc",
			WWN:        "wow",
		},
	}
	volumeInfo := state.VolumeInfo{
		VolumeId: "543554ff-3b88-43b9-83fc-4d69de28490b",
	}
	atachmentInfo := state.VolumeAttachmentInfo{}
	planBlockInfo := blockdevice.BlockDevice{}
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), blockDevices, volumeInfo, atachmentInfo, planBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "sdb",
		SerialId:   "543554ff-3b88-4",
	})
}

func (s *BlockDeviceSuite) TestBlockDeviceMatchingHardwareID(c *tc.C) {
	blockDevices := []blockdevice.BlockDevice{
		{
			DeviceName: "sdb",
			HardwareId: "ide-543554ff-3b88-4",
		},
		{
			DeviceName: "sdc",
		},
	}
	volumeInfo := state.VolumeInfo{
		HardwareId: "ide-543554ff-3b88-4",
	}
	atachmentInfo := state.VolumeAttachmentInfo{}
	planBlockInfo := blockdevice.BlockDevice{}
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), blockDevices, volumeInfo, atachmentInfo, planBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "sdb",
		HardwareId: "ide-543554ff-3b88-4",
	})
}

func (s *BlockDeviceSuite) TestBlockDevicesAWS(c *tc.C) {
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), awsTestBlockDevices, awsTestVolumeInfo, awsTestAttachmentInfo, awsTestPlanBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "nvme0n1",
		DeviceLinks: []string{
			"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol091bc356f4cc7661c",
			"/dev/disk/by-id/nvme-nvme.1d0f-766f6c3039316263333536663463633736363163-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
			"/dev/disk/by-path/pci-0000:00:1f.0-nvme-1",
		},
		WWN:      "nvme.1d0f-766f6c3039316263333536663463633736363163-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
		SizeMiB:  0x800,
		SerialId: "Amazon Elastic Block Store_vol091bc356f4cc7661c",
	})
}

var (
	awsTestBlockDevices = []blockdevice.BlockDevice{{
		DeviceName:     "loop0",
		SizeMiB:        0x59,
		FilesystemType: "squashfs",
		InUse:          true,
		MountPoint:     "/snap/core/7713",
	}, {
		DeviceName:     "loop1",
		SizeMiB:        0x11,
		FilesystemType: "squashfs",
		InUse:          true,
		MountPoint:     "/snap/amazon-ssm-agent/1480",
	}, {
		DeviceName: "nvme0n1",
		DeviceLinks: []string{
			"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol091bc356f4cc7661c",
			"/dev/disk/by-id/nvme-nvme.1d0f-766f6c3039316263333536663463633736363163-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
			"/dev/disk/by-path/pci-0000:00:1f.0-nvme-1",
		},
		WWN:      "nvme.1d0f-766f6c3039316263333536663463633736363163-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
		SizeMiB:  0x800,
		SerialId: "Amazon Elastic Block Store_vol091bc356f4cc7661c",
	}, {
		DeviceName: "nvme1n1",
		DeviceLinks: []string{
			"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol04aa6a883e0e79a77",
			"/dev/disk/by-id/nvme-nvme.1d0f-766f6c3034616136613838336530653739613737-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
			"/dev/disk/by-path/pci-0000:00:04.0-nvme-1",
		},
		WWN:      "nvme.1d0f-766f6c3034616136613838336530653739613737-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
		SizeMiB:  0x2000,
		InUse:    true,
		SerialId: "Amazon Elastic Block Store_vol04aa6a883e0e79a77",
	}, {
		DeviceName: "nvme1n1p1",
		DeviceLinks: []string{
			"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol04aa6a883e0e79a77-part1",
			"/dev/disk/by-id/nvme-nvme.1d0f-766f6c3034616136613838336530653739613737-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001-part1",
			"/dev/disk/by-label/cloudimg-rootfs",
			"/dev/disk/by-partuuid/eeaf5908-01",
			"/dev/disk/by-path/pci-0000:00:04.0-nvme-1-part1",
			"/dev/disk/by-uuid/651cda91-e465-4685-b697-67aa07181279",
		},
		Label:          "cloudimg-rootfs",
		UUID:           "651cda91-e465-4685-b697-67aa07181279",
		WWN:            "nvme.1d0f-766f6c3034616136613838336530653739613737-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
		SizeMiB:        0x1ffe,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
		SerialId:       "Amazon Elastic Block Store_vol04aa6a883e0e79a77",
	}, {
		DeviceName: "nvme2n1",
		DeviceLinks: []string{
			"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol08a1b32d17fdda355",
			"/dev/disk/by-id/nvme-nvme.1d0f-766f6c3038613162333264313766646461333535-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
			"/dev/disk/by-path/pci-0000:00:1e.0-nvme-1",
		},
		WWN:      "nvme.1d0f-766f6c3038613162333264313766646461333535-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
		SizeMiB:  0xc00,
		SerialId: "Amazon Elastic Block Store_vol08a1b32d17fdda355",
	}, {
		DeviceName: "nvme3n1",
		DeviceLinks: []string{
			"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0389eb49d7a7ab355",
			"/dev/disk/by-id/nvme-nvme.1d0f-766f6c3033383965623439643761376162333535-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
			"/dev/disk/by-path/pci-0000:00:1d.0-nvme-1",
		},
		WWN:      "nvme.1d0f-766f6c3033383965623439643761376162333535-416d617a6f6e20456c617374696320426c6f636b2053746f7265-00000001",
		SizeMiB:  0x800,
		SerialId: "Amazon Elastic Block Store_vol0389eb49d7a7ab355",
	}}
	awsTestPlanBlockInfo  = blockdevice.BlockDevice{}
	awsTestVolumeInfo     = state.VolumeInfo{Size: 0x800, Pool: "ebs", VolumeId: "vol-091bc356f4cc7661c", Persistent: true}
	awsTestAttachmentInfo = state.VolumeAttachmentInfo{DeviceName: "xvdf", DeviceLink: "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol091bc356f4cc7661c"}
)

func (s *BlockDeviceSuite) TestBlockDevicesGCE(c *tc.C) {
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), gceTestBlockDevices, gceTestVolumeInfo, gceTestAttachmentInfo, gceTestPlanBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "sdd",
		DeviceLinks: []string{
			"/dev/disk/by-id/google-us-east1-d-5005808815463186635",
			"/dev/disk/by-id/scsi-0Google_PersistentDisk_us-east1-d-5005808815463186635",
			"/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:4:0",
		},
		HardwareId: "scsi-0Google_PersistentDisk_us-east1-d-5005808815463186635",
		BusAddress: "scsi@0:0.4.0",
		SizeMiB:    0x2800,
		SerialId:   "0Google_PersistentDisk_us-east1-d-5005808815463186635",
	})
}

func (s *BlockDeviceSuite) TestBlockDevicesGCEPreferUUID(c *tc.C) {
	blockDeviceInfo, ok := storagecommon.MatchingFilesystemBlockDevice(c.Context(), gceTestBlockDevices, gceTestVolumeInfo, gceTestAttachmentInfoForUUID, gceTestPlanBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "sda1",
		DeviceLinks: []string{
			"/dev/disk/by-id/google-persistent-disk-0-part1",
			"/dev/disk/by-id/scsi-0Google_PersistentDisk_persistent-disk-0-part1",
			"/dev/disk/by-label/cloudimg-rootfs",
			"/dev/disk/by-partuuid/8c3230b8-1ecf-45d9-a6c8-41f4bc51a849",
			"/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:1:0-part1",
			"/dev/disk/by-uuid/27514291-b7f6-4b83-bc8a-07c7d7467218",
		},
		Label:          "cloudimg-rootfs",
		UUID:           "27514291-b7f6-4b83-bc8a-07c7d7467218",
		HardwareId:     "scsi-0Google_PersistentDisk_persistent-disk-0",
		BusAddress:     "scsi@0:0.1.0",
		SizeMiB:        0x2790,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
		SerialId:       "0Google_PersistentDisk_persistent-disk-0",
	})
}

var (
	gceTestBlockDevices          = []blockdevice.BlockDevice{{DeviceName: "loop0", SizeMiB: 0x59, FilesystemType: "squashfs", InUse: true, MountPoint: "/snap/core/7713"}, {DeviceName: "loop1", SizeMiB: 0x42, FilesystemType: "squashfs", InUse: true, MountPoint: "/snap/google-cloud-sdk/102"}, {DeviceName: "sda", DeviceLinks: []string{"/dev/disk/by-id/google-persistent-disk-0", "/dev/disk/by-id/scsi-0Google_PersistentDisk_persistent-disk-0", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:1:0"}, HardwareId: "scsi-0Google_PersistentDisk_persistent-disk-0", BusAddress: "scsi@0:0.1.0", SizeMiB: 0x2800, InUse: true, SerialId: "0Google_PersistentDisk_persistent-disk-0"}, {DeviceName: "sda1", DeviceLinks: []string{"/dev/disk/by-id/google-persistent-disk-0-part1", "/dev/disk/by-id/scsi-0Google_PersistentDisk_persistent-disk-0-part1", "/dev/disk/by-label/cloudimg-rootfs", "/dev/disk/by-partuuid/8c3230b8-1ecf-45d9-a6c8-41f4bc51a849", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:1:0-part1", "/dev/disk/by-uuid/27514291-b7f6-4b83-bc8a-07c7d7467218"}, Label: "cloudimg-rootfs", UUID: "27514291-b7f6-4b83-bc8a-07c7d7467218", HardwareId: "scsi-0Google_PersistentDisk_persistent-disk-0", BusAddress: "scsi@0:0.1.0", SizeMiB: 0x2790, FilesystemType: "ext4", InUse: true, MountPoint: "/", SerialId: "0Google_PersistentDisk_persistent-disk-0"}, {DeviceName: "sda14", DeviceLinks: []string{"/dev/disk/by-id/google-persistent-disk-0-part14", "/dev/disk/by-id/scsi-0Google_PersistentDisk_persistent-disk-0-part14", "/dev/disk/by-partuuid/d82926ca-95f8-46fe-ab94-61bb6cc2a879", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:1:0-part14"}, HardwareId: "scsi-0Google_PersistentDisk_persistent-disk-0", BusAddress: "scsi@0:0.1.0", SizeMiB: 0x4, SerialId: "0Google_PersistentDisk_persistent-disk-0"}, {DeviceName: "sda15", DeviceLinks: []string{"/dev/disk/by-id/google-persistent-disk-0-part15", "/dev/disk/by-id/scsi-0Google_PersistentDisk_persistent-disk-0-part15", "/dev/disk/by-label/UEFI", "/dev/disk/by-partuuid/264a576d-0211-45fa-9bdd-e674c08517f4", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:1:0-part15", "/dev/disk/by-uuid/9889-C357"}, Label: "UEFI", UUID: "9889-C357", HardwareId: "scsi-0Google_PersistentDisk_persistent-disk-0", BusAddress: "scsi@0:0.1.0", SizeMiB: 0x6a, FilesystemType: "vfat", InUse: true, MountPoint: "/boot/efi", SerialId: "0Google_PersistentDisk_persistent-disk-0"}, {DeviceName: "sdb", DeviceLinks: []string{"/dev/disk/by-id/google-us-east1-d-9082123458182365433", "/dev/disk/by-id/scsi-0Google_PersistentDisk_us-east1-d-9082123458182365433", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:2:0"}, HardwareId: "scsi-0Google_PersistentDisk_us-east1-d-9082123458182365433", BusAddress: "scsi@0:0.2.0", SizeMiB: 0x2800, SerialId: "0Google_PersistentDisk_us-east1-d-9082123458182365433"}, {DeviceName: "sdc", DeviceLinks: []string{"/dev/disk/by-id/google-us-east1-d-2880464023067017457", "/dev/disk/by-id/scsi-0Google_PersistentDisk_us-east1-d-2880464023067017457", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:3:0"}, HardwareId: "scsi-0Google_PersistentDisk_us-east1-d-2880464023067017457", BusAddress: "scsi@0:0.3.0", SizeMiB: 0x2800, SerialId: "0Google_PersistentDisk_us-east1-d-2880464023067017457"}, {DeviceName: "sdd", DeviceLinks: []string{"/dev/disk/by-id/google-us-east1-d-5005808815463186635", "/dev/disk/by-id/scsi-0Google_PersistentDisk_us-east1-d-5005808815463186635", "/dev/disk/by-path/pci-0000:00:03.0-scsi-0:0:4:0"}, HardwareId: "scsi-0Google_PersistentDisk_us-east1-d-5005808815463186635", BusAddress: "scsi@0:0.4.0", SizeMiB: 0x2800, SerialId: "0Google_PersistentDisk_us-east1-d-5005808815463186635"}}
	gceTestPlanBlockInfo         = blockdevice.BlockDevice{}
	gceTestVolumeInfo            = state.VolumeInfo{Size: 0x2800, Pool: "gce", VolumeId: "us-east1-d--515cb1ad-5d23-4d53-8cc1-b79c75a03908", Persistent: true}
	gceTestAttachmentInfo        = state.VolumeAttachmentInfo{DeviceLink: "/dev/disk/by-id/google-us-east1-d-5005808815463186635", ReadOnly: false, PlanInfo: (*state.VolumeAttachmentPlanInfo)(nil)}
	gceTestAttachmentInfoForUUID = state.VolumeAttachmentInfo{DeviceLink: "/dev/disk/by-id/google-persistent-disk-0", ReadOnly: false, PlanInfo: (*state.VolumeAttachmentPlanInfo)(nil)}
)

func (s *BlockDeviceSuite) TestBlockDevicesOpenStack(c *tc.C) {
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), osTestBlockDevices, osTestVolumeInfo, osTestAttachmentInfo, osTestPlanBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "vdd",
		DeviceLinks: []string{
			"/dev/disk/by-id/virtio-6a905f6b-e5b6-49e9-b",
			"/dev/disk/by-path/pci-0000:00:09.0",
			"/dev/disk/by-path/virtio-pci-0000:00:09.0",
		},
		SizeMiB:  0xc00,
		SerialId: "6a905f6b-e5b6-49e9-b",
	})
}

var (
	osTestBlockDevices   = []blockdevice.BlockDevice{{DeviceName: "vda", DeviceLinks: []string{"/dev/disk/by-path/pci-0000:00:05.0", "/dev/disk/by-path/virtio-pci-0000:00:05.0"}, SizeMiB: 0x2800, InUse: true}, {DeviceName: "vda1", DeviceLinks: []string{"/dev/disk/by-label/cloudimg-rootfs", "/dev/disk/by-parttypeuuid/0fc63daf-8483-4772-8e79-3d69d8477de4.1ee47053-dace-47ff-b708-d10b148face7", "/dev/disk/by-partuuid/1ee47053-dace-47ff-b708-d10b148face7", "/dev/disk/by-path/pci-0000:00:05.0-part1", "/dev/disk/by-path/virtio-pci-0000:00:05.0-part1", "/dev/disk/by-uuid/4110798a-f017-4fbb-87a3-d1ae56309905"}, Label: "cloudimg-rootfs", UUID: "4110798a-f017-4fbb-87a3-d1ae56309905", SizeMiB: 0x2790, FilesystemType: "ext4", InUse: true, MountPoint: "/"}, {DeviceName: "vda14", DeviceLinks: []string{"/dev/disk/by-parttypeuuid/21686148-6449-6e6f-744e-656564454649.af16be47-13f7-4ec6-bfad-81a96391e007", "/dev/disk/by-partuuid/af16be47-13f7-4ec6-bfad-81a96391e007", "/dev/disk/by-path/pci-0000:00:05.0-part14", "/dev/disk/by-path/virtio-pci-0000:00:05.0-part14"}, SizeMiB: 0x4}, {DeviceName: "vda15", DeviceLinks: []string{"/dev/disk/by-label/UEFI", "/dev/disk/by-parttypeuuid/c12a7328-f81f-11d2-ba4b-00a0c93ec93b.95e45c83-66e7-4049-bbb3-184e71c78ab0", "/dev/disk/by-partuuid/95e45c83-66e7-4049-bbb3-184e71c78ab0", "/dev/disk/by-path/pci-0000:00:05.0-part15", "/dev/disk/by-path/virtio-pci-0000:00:05.0-part15", "/dev/disk/by-uuid/240B-0762"}, Label: "UEFI", UUID: "240B-0762", SizeMiB: 0x6a, FilesystemType: "vfat", InUse: true, MountPoint: "/boot/efi"}, {DeviceName: "vdb", DeviceLinks: []string{"/dev/disk/by-id/lvm-pv-uuid-x0BACK-yGe4-rzdr-HUbU-C8n7-0RuM-lLLYds", "/dev/disk/by-id/virtio-084eff6a-6c73-4aab-a", "/dev/disk/by-path/pci-0000:00:07.0", "/dev/disk/by-path/virtio-pci-0000:00:07.0"}, UUID: "x0BACK-yGe4-rzdr-HUbU-C8n7-0RuM-lLLYds", SizeMiB: 0x800, FilesystemType: "LVM2_member", InUse: true, SerialId: "084eff6a-6c73-4aab-a"}, {DeviceName: "vdc", DeviceLinks: []string{"/dev/disk/by-id/lvm-pv-uuid-PyQyVT-ASna-kCli-8BzD-Haq8-qHPQ-JUOdPz", "/dev/disk/by-id/virtio-e15e24cf-eafb-4759-9", "/dev/disk/by-path/pci-0000:00:08.0", "/dev/disk/by-path/virtio-pci-0000:00:08.0"}, UUID: "PyQyVT-ASna-kCli-8BzD-Haq8-qHPQ-JUOdPz", SizeMiB: 0x800, FilesystemType: "LVM2_member", InUse: true, SerialId: "e15e24cf-eafb-4759-9"}, {DeviceName: "vdd", DeviceLinks: []string{"/dev/disk/by-id/virtio-6a905f6b-e5b6-49e9-b", "/dev/disk/by-path/pci-0000:00:09.0", "/dev/disk/by-path/virtio-pci-0000:00:09.0"}, SizeMiB: 0xc00, SerialId: "6a905f6b-e5b6-49e9-b"}}
	osTestPlanBlockInfo  = blockdevice.BlockDevice{}
	osTestVolumeInfo     = state.VolumeInfo{Size: 0xc00, Pool: "cinder", VolumeId: "6a905f6b-e5b6-49e9-b2dd-96a60f35befe", Persistent: true}
	osTestAttachmentInfo = state.VolumeAttachmentInfo{DeviceName: "vdd"}
)

func (s *BlockDeviceSuite) TestBlockDevicesOCI(c *tc.C) {
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), ociTestBlockDevices, ociTestVolumeInfo, ociTestAttachmentInfo, ociTestPlanBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "loop2",
		SizeMiB:    0x800,
	})
}

var (
	ociTestBlockDevices   = []blockdevice.BlockDevice{{DeviceName: "loop0", SizeMiB: 0x58, FilesystemType: "squashfs", InUse: true, MountPoint: "/snap/core/7396"}, {DeviceName: "loop1", SizeMiB: 0xe, FilesystemType: "squashfs", InUse: true, MountPoint: "/snap/oracle-cloud-agent/4"}, {DeviceName: "loop2", SizeMiB: 0x800}, {DeviceName: "loop3", SizeMiB: 0x800}, {DeviceName: "loop4", SizeMiB: 0xc00}, {DeviceName: "sda", DeviceLinks: []string{"/dev/disk/by-id/scsi-360415622505749fcaf8d9b18658682bb", "/dev/disk/by-id/wwn-0x60415622505749fcaf8d9b18658682bb", "/dev/disk/by-path/pci-0000:00:04.0-scsi-0:0:0:1", "/dev/oracleoci/oraclevda"}, HardwareId: "scsi-360415622505749fcaf8d9b18658682bb", WWN: "0x60415622505749fcaf8d9b18658682bb", BusAddress: "scsi@2:0.0.1", SizeMiB: 0xc800, InUse: true, SerialId: "360415622505749fcaf8d9b18658682bb"}, {DeviceName: "sda1", DeviceLinks: []string{"/dev/disk/by-id/scsi-360415622505749fcaf8d9b18658682bb-part1", "/dev/disk/by-id/wwn-0x60415622505749fcaf8d9b18658682bb-part1", "/dev/disk/by-label/cloudimg-rootfs", "/dev/disk/by-partuuid/e40ca084-a894-4cf9-afc2-5d824b874d20", "/dev/disk/by-path/pci-0000:00:04.0-scsi-0:0:0:1-part1", "/dev/disk/by-uuid/15993e31-3f38-4b9f-bdeb-74e0541498d0", "/dev/oracleoci/oraclevda1"}, Label: "cloudimg-rootfs", UUID: "15993e31-3f38-4b9f-bdeb-74e0541498d0", HardwareId: "scsi-360415622505749fcaf8d9b18658682bb", WWN: "0x60415622505749fcaf8d9b18658682bb", BusAddress: "scsi@2:0.0.1", SizeMiB: 0xc790, FilesystemType: "ext4", InUse: true, MountPoint: "/", SerialId: "360415622505749fcaf8d9b18658682bb"}, {DeviceName: "sda14", DeviceLinks: []string{"/dev/disk/by-id/scsi-360415622505749fcaf8d9b18658682bb-part14", "/dev/disk/by-id/wwn-0x60415622505749fcaf8d9b18658682bb-part14", "/dev/disk/by-partuuid/e4fc0d03-d104-4b96-bcb8-34b8371dda96", "/dev/disk/by-path/pci-0000:00:04.0-scsi-0:0:0:1-part14", "/dev/oracleoci/oraclevda14"}, HardwareId: "scsi-360415622505749fcaf8d9b18658682bb", WWN: "0x60415622505749fcaf8d9b18658682bb", BusAddress: "scsi@2:0.0.1", SizeMiB: 0x4, SerialId: "360415622505749fcaf8d9b18658682bb"}, {DeviceName: "sda15", DeviceLinks: []string{"/dev/disk/by-id/scsi-360415622505749fcaf8d9b18658682bb-part15", "/dev/disk/by-id/wwn-0x60415622505749fcaf8d9b18658682bb-part15", "/dev/disk/by-label/UEFI", "/dev/disk/by-partuuid/3128f459-5457-4216-bbd9-4008c61d5b26", "/dev/disk/by-path/pci-0000:00:04.0-scsi-0:0:0:1-part15", "/dev/disk/by-uuid/323C-AF60", "/dev/oracleoci/oraclevda15"}, Label: "UEFI", UUID: "323C-AF60", HardwareId: "scsi-360415622505749fcaf8d9b18658682bb", WWN: "0x60415622505749fcaf8d9b18658682bb", BusAddress: "scsi@2:0.0.1", SizeMiB: 0x6a, FilesystemType: "vfat", InUse: true, MountPoint: "/boot/efi", SerialId: "360415622505749fcaf8d9b18658682bb"}}
	ociTestPlanBlockInfo  = blockdevice.BlockDevice{}
	ociTestVolumeInfo     = state.VolumeInfo{Size: 0x800, Pool: "loop", VolumeId: "volume-5-3"}
	ociTestAttachmentInfo = state.VolumeAttachmentInfo{DeviceName: "loop2"}
)

func (s *BlockDeviceSuite) TestBlockDevicesVSphere(c *tc.C) {
	blockDeviceInfo, ok := storagecommon.MatchingVolumeBlockDevice(c.Context(), vsphereTestBlockDevices, vsphereTestVolumeInfo, vsphereTestAttachmentInfo, vsphereTestPlanBlockInfo)
	c.Assert(ok, tc.IsTrue)
	c.Assert(blockDeviceInfo, tc.DeepEquals, &blockdevice.BlockDevice{
		DeviceName: "loop0",
		SizeMiB:    0x800,
	})
}

var (
	vsphereTestBlockDevices   = []blockdevice.BlockDevice{{DeviceName: "loop0", SizeMiB: 0x800}, {DeviceName: "loop1", SizeMiB: 0x800}, {DeviceName: "loop2", SizeMiB: 0xc00}, {DeviceName: "sda", DeviceLinks: []string{"/dev/disk/by-id/scsi-36000c29b30fd2a905a8e395b92434bd8", "/dev/disk/by-id/wwn-0x6000c29b30fd2a905a8e395b92434bd8", "/dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:0:0"}, HardwareId: "scsi-36000c29b30fd2a905a8e395b92434bd8", WWN: "0x6000c29b30fd2a905a8e395b92434bd8", BusAddress: "scsi@2:0.0.0", SizeMiB: 0x2800, InUse: true, SerialId: "36000c29b30fd2a905a8e395b92434bd8"}, {DeviceName: "sda1", DeviceLinks: []string{"/dev/disk/by-id/scsi-36000c29b30fd2a905a8e395b92434bd8-part1", "/dev/disk/by-id/wwn-0x6000c29b30fd2a905a8e395b92434bd8-part1", "/dev/disk/by-label/cloudimg-rootfs", "/dev/disk/by-partuuid/1ee47053-dace-47ff-b708-d10b148face7", "/dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:0:0-part1", "/dev/disk/by-uuid/4110798a-f017-4fbb-87a3-d1ae56309905"}, Label: "cloudimg-rootfs", UUID: "4110798a-f017-4fbb-87a3-d1ae56309905", HardwareId: "scsi-36000c29b30fd2a905a8e395b92434bd8", WWN: "0x6000c29b30fd2a905a8e395b92434bd8", BusAddress: "scsi@2:0.0.0", SizeMiB: 0x2790, FilesystemType: "ext4", InUse: true, MountPoint: "/", SerialId: "36000c29b30fd2a905a8e395b92434bd8"}, {DeviceName: "sda14", DeviceLinks: []string{"/dev/disk/by-id/scsi-36000c29b30fd2a905a8e395b92434bd8-part14", "/dev/disk/by-id/wwn-0x6000c29b30fd2a905a8e395b92434bd8-part14", "/dev/disk/by-partuuid/af16be47-13f7-4ec6-bfad-81a96391e007", "/dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:0:0-part14"}, HardwareId: "scsi-36000c29b30fd2a905a8e395b92434bd8", WWN: "0x6000c29b30fd2a905a8e395b92434bd8", BusAddress: "scsi@2:0.0.0", SizeMiB: 0x4, SerialId: "36000c29b30fd2a905a8e395b92434bd8"}, {DeviceName: "sda15", DeviceLinks: []string{"/dev/disk/by-id/scsi-36000c29b30fd2a905a8e395b92434bd8-part15", "/dev/disk/by-id/wwn-0x6000c29b30fd2a905a8e395b92434bd8-part15", "/dev/disk/by-label/UEFI", "/dev/disk/by-partuuid/95e45c83-66e7-4049-bbb3-184e71c78ab0", "/dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:0:0-part15", "/dev/disk/by-uuid/240B-0762"}, Label: "UEFI", UUID: "240B-0762", HardwareId: "scsi-36000c29b30fd2a905a8e395b92434bd8", WWN: "0x6000c29b30fd2a905a8e395b92434bd8", BusAddress: "scsi@2:0.0.0", SizeMiB: 0x6a, FilesystemType: "vfat", InUse: true, MountPoint: "/boot/efi", SerialId: "36000c29b30fd2a905a8e395b92434bd8"}}
	vsphereTestPlanBlockInfo  = blockdevice.BlockDevice{}
	vsphereTestVolumeInfo     = state.VolumeInfo{Size: 0x800, Pool: "loop", VolumeId: "volume-5-6"}
	vsphereTestAttachmentInfo = state.VolumeAttachmentInfo{DeviceName: "loop0"}
)
