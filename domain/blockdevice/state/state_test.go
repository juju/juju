// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) TestBlockDevicesNone(c *tc.C) {
	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *stateSuite) createMachine(c *tc.C, machineId string) string {
	return s.createMachineWithLife(c, machineId, life.Alive)
}

func (s *stateSuite) createMachineWithLife(c *tc.C, name string, life life.Life) string {
	db := s.DB()

	netNodeUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(context.Background(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	c.Assert(err, jc.ErrorIsNil)
	machineUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(context.Background(), `
INSERT INTO machine (uuid, life_id, name, net_node_uuid)
VALUES (?, ?, ?, ?)
`, machineUUID, life, name, netNodeUUID)
	c.Assert(err, jc.ErrorIsNil)
	return machineUUID
}

func (s *stateSuite) insertBlockDevice(c *tc.C, bd blockdevice.BlockDevice, blockDeviceUUID, machineUUID string) {
	db := s.DB()

	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := db.ExecContext(context.Background(), `
INSERT INTO block_device (uuid, machine_uuid, name, label, device_uuid, hardware_id, wwn, bus_address, serial_id, mount_point, filesystem_type_id, Size_mib, in_use)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 4, ?, ?)
`, blockDeviceUUID, machineUUID, bd.DeviceName, bd.Label, bd.UUID, bd.HardwareId, bd.WWN, bd.BusAddress, bd.SerialId, bd.MountPoint, bd.SizeMiB, inUse)
	c.Assert(err, jc.ErrorIsNil)

	for _, link := range bd.DeviceLinks {
		_, err = db.ExecContext(context.Background(), `
INSERT INTO block_device_link_device (block_device_uuid, name)
VALUES (?, ?)
`, blockDeviceUUID, link)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *stateSuite) TestBlockDevicesOne(c *tc.C) {
	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	blockDeviceUUID := uuid.MustNewUUID().String()
	machineUUID := s.createMachine(c, "666")
	s.insertBlockDevice(c, bd, blockDeviceUUID, machineUUID)

	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []blockdevice.BlockDevice{bd})
}

func (s *stateSuite) TestBlockDevicesMany(c *tc.C) {
	machineUUID := s.createMachine(c, "666")

	bd1 := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bd2 := blockdevice.BlockDevice{
		DeviceName:     "name-667",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-667",
		UUID:           "device-667",
		HardwareId:     "hardware-667",
		WWN:            "wwn-667",
		BusAddress:     "bus-667",
		SizeMiB:        667,
		FilesystemType: "btrfs",
		MountPoint:     "mount-667",
		SerialId:       "serial-667",
	}
	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd1, blockDevice1UUID, machineUUID)
	blockDevice2UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd2, blockDevice2UUID, machineUUID)

	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []blockdevice.BlockDevice{bd1, bd2})
}

func (s *stateSuite) TestBlockDevicesFilersOnMachine(c *tc.C) {
	machine1UUID := s.createMachine(c, "666")
	machine2UUID := s.createMachine(c, "667")

	bd1 := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bd2 := blockdevice.BlockDevice{
		DeviceName:     "name-667",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-667",
		UUID:           "device-667",
		HardwareId:     "hardware-667",
		WWN:            "wwn-667",
		BusAddress:     "bus-667",
		SizeMiB:        667,
		FilesystemType: "btrfs",
		MountPoint:     "mount-667",
		SerialId:       "serial-667",
	}
	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd1, blockDevice1UUID, machine1UUID)
	blockDevice2UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd2, blockDevice2UUID, machine2UUID)

	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "667")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []blockdevice.BlockDevice{bd2})
}

func (s *stateSuite) TestMachineBlockDevices(c *tc.C) {
	machine1UUID := s.createMachine(c, "666")
	machine2UUID := s.createMachine(c, "667")

	bd1 := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bd2 := blockdevice.BlockDevice{
		DeviceName:     "name-667",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-667",
		UUID:           "device-667",
		HardwareId:     "hardware-667",
		WWN:            "wwn-667",
		BusAddress:     "bus-667",
		SizeMiB:        667,
		FilesystemType: "btrfs",
		MountPoint:     "mount-667",
		SerialId:       "serial-667",
	}
	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd1, blockDevice1UUID, machine1UUID)
	blockDevice2UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd2, blockDevice2UUID, machine2UUID)

	result, err := NewState(s.TxnRunnerFactory()).MachineBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []blockdevice.MachineBlockDevice{
		{MachineId: "666", BlockDevice: bd1},
		{MachineId: "667", BlockDevice: bd2},
	})
}

func (s *stateSuite) TestSetMachineBlockDevicesDeadMachine(c *tc.C) {
	s.createMachineWithLife(c, "666", 2)

	bd := blockdevice.BlockDevice{}

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, tc.ErrorMatches, `cannot update block devices on dead machine "666"`)
}

func (s *stateSuite) TestSetMachineBlockDevicesMissingMachine(c *tc.C) {
	bd := blockdevice.BlockDevice{}

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, tc.ErrorMatches, `machine "666" not found`)
}

func (s *stateSuite) TestSetMachineBlockDevicesBadFilesystemType(c *tc.C) {
	s.createMachine(c, "666")

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		FilesystemType: "foo",
	}

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, tc.ErrorMatches, `updating block devices on machine "666".*: filesystem type "foo" for block device "name-666" not valid`)
}

func (s *stateSuite) TestSetMachineBlockDevices(c *tc.C) {
	s.createMachine(c, "666")

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []blockdevice.BlockDevice{bd})

	// Idempotent.
	err = NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	result, err = NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []blockdevice.BlockDevice{bd})
}

func (s *stateSuite) TestSetMachineBlockDevicesUpdates(c *tc.C) {
	s.createMachine(c, "666")

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []blockdevice.BlockDevice{bd})

	bd.DeviceLinks = []string{"dev_link3", "dev_link4"}
	bd.DeviceName = "device-667"
	err = NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	result, err = NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []blockdevice.BlockDevice{bd})

	db := s.DB()
	var num int

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 1)

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device_link_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 2)
}

func (s *stateSuite) TestSetMachineBlockDevicesReplacesExisting(c *tc.C) {
	s.createMachine(c, "666")

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bd2 := blockdevice.BlockDevice{
		DeviceName:     "name-667",
		DeviceLinks:    []string{"dev_link2", "dev_link3"},
		Label:          "label-667",
		UUID:           "device-667",
		HardwareId:     "hardware-667",
		WWN:            "wwn-667",
		BusAddress:     "bus-667",
		SizeMiB:        667,
		FilesystemType: "btrfs",
		MountPoint:     "mount-667",
		SerialId:       "serial-667",
	}

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd, bd2)
	c.Assert(err, jc.ErrorIsNil)
	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []blockdevice.BlockDevice{bd, bd2})

	err = NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666", bd)
	c.Assert(err, jc.ErrorIsNil)
	result, err = NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []blockdevice.BlockDevice{bd})

	db := s.DB()
	var num int

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 1)

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device_link_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 2)
}

func (s *stateSuite) TestSetMachineBlockDevicesToEmpty(c *tc.C) {
	machineUUID := s.createMachine(c, "666")

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}

	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd, blockDevice1UUID, machineUUID)

	err := NewState(s.TxnRunnerFactory()).SetMachineBlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	db := s.DB()
	var num int

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 0)

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device_link_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 0)
}

func (s *stateSuite) TestRemoveMachineBlockDevices(c *tc.C) {
	machineUUID := s.createMachine(c, "666")

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		DeviceLinks:    []string{"dev_link1", "dev_link2"},
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}

	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd, blockDevice1UUID, machineUUID)

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return RemoveMachineBlockDevices(context.Background(), tx, machineUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := NewState(s.TxnRunnerFactory()).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	db := s.DB()
	var num int

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 0)

	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM block_device_link_device").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, tc.Equals, 0)
}
