// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestBlockDevicesNone(c *tc.C) {
	machineUUID := s.createMachine(c, "666")

	st := NewState(s.TxnRunnerFactory())
	result, err := st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *stateSuite) createMachine(c *tc.C, machineId string) machine.UUID {
	return s.createMachineWithLife(c, machineId, life.Alive)
}

func (s *stateSuite) createMachineWithLife(c *tc.C, name string, life life.Life) machine.UUID {
	netNodeUUID := uuid.MustNewUUID().String()
	_, err := s.DB().Exec("INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	machineUUID := tc.Must(c, machine.NewUUID)
	_, err = s.DB().Exec(`
INSERT INTO machine (uuid, life_id, name, net_node_uuid)
VALUES (?, ?, ?, ?)
`, machineUUID, life, name, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func (s *stateSuite) insertBlockDevice(
	c *tc.C, bd blockdevice.BlockDevice,
	blockDeviceUUID string, machineUUID machine.UUID,
) {
	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := s.DB().Exec(`
INSERT INTO block_device (
	uuid, machine_uuid, name, filesystem_label,
	filesystem_uuid, hardware_id, wwn, bus_address, serial_id,
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
}

func (s *stateSuite) TestBlockDevicesOne(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	bd := blockdevice.BlockDevice{
		DeviceName:      "name-666",
		DeviceLinks:     []string{"dev_link1", "dev_link2"},
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
	blockDeviceUUID := uuid.MustNewUUID().String()
	machineUUID := s.createMachine(c, "666")
	s.insertBlockDevice(c, bd, blockDeviceUUID, machineUUID)

	result, err := st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]blockdevice.BlockDevice{
		blockDeviceUUID: bd,
	})
}

func (s *stateSuite) TestBlockDevicesMany(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machineUUID := s.createMachine(c, "666")

	bd1 := blockdevice.BlockDevice{
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
	bd2 := blockdevice.BlockDevice{
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
	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd1, blockDevice1UUID, machineUUID)
	blockDevice2UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd2, blockDevice2UUID, machineUUID)

	result, err := st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]blockdevice.BlockDevice{
		blockDevice1UUID: bd1,
		blockDevice2UUID: bd2,
	})
}

func (s *stateSuite) TestBlockDevicesFilersOnMachine(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machine1UUID := s.createMachine(c, "666")
	machine2UUID := s.createMachine(c, "667")

	bd1 := blockdevice.BlockDevice{
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
	bd2 := blockdevice.BlockDevice{
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
	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd1, blockDevice1UUID, machine1UUID)
	blockDevice2UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd2, blockDevice2UUID, machine2UUID)

	result, err := st.BlockDevices(c.Context(), machine2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]blockdevice.BlockDevice{
		blockDevice2UUID: bd2,
	})
}

func (s *stateSuite) TestMachineBlockDevices(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machine1UUID := s.createMachine(c, "666")
	machine2UUID := s.createMachine(c, "667")

	bd1 := blockdevice.BlockDevice{
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
	bd2 := blockdevice.BlockDevice{
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
	blockDevice1UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd1, blockDevice1UUID, machine1UUID)
	blockDevice2UUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd2, blockDevice2UUID, machine2UUID)

	result, err := st.MachineBlockDevices(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[machine.Name][]blockdevice.BlockDevice{
		"666": {bd1},
		"667": {bd2},
	})
}

func (s *stateSuite) TestUpdateMachineBlockDevicesDeadMachine(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	uuid := s.createMachineWithLife(c, "666", 2)

	err := st.UpdateMachineBlockDevices(
		c.Context(), uuid, nil, nil, nil)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineIsDead)
}

func (s *stateSuite) TestUpdateMachineBlockDevicesMissingMachine(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machineUUID := tc.Must(c, machine.NewUUID)

	err := st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, nil, nil, nil)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestUpdateBlockDevices(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machineUUID := s.createMachine(c, "666")

	added := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName:      "name-666",
			DeviceLinks:     []string{"dev_link1", "dev_link2"},
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
		},
	}

	err := st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, added, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	result, err := st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, added)
}

func (s *stateSuite) TestUpdateBlockDevicesRemoves(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machineUUID := s.createMachine(c, "666")

	added := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName:      "name-666",
			DeviceLinks:     []string{"dev_link1", "dev_link2"},
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
		},
	}
	err := st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, added, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, added)

	err = st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, nil, nil, []string{"a"})
	c.Assert(err, tc.ErrorIsNil)

	result, err = st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *stateSuite) TestUpdateBlockDevicesUpdates(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	machineUUID := s.createMachine(c, "666")

	added := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName:      "name-666",
			DeviceLinks:     []string{"dev_link1", "dev_link2"},
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
		},
	}
	err := st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, added, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, added)

	updated := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName:      "name-666_b",
			DeviceLinks:     []string{"dev_link1_b", "dev_link2_b"},
			FilesystemLabel: "label-666_b",
			FilesystemUUID:  "device-666_b",
			HardwareId:      "hardware-666_b",
			WWN:             "wwn-666_b",
			BusAddress:      "bus-666_b",
			SizeMiB:         6666,
			FilesystemType:  "ext4",
			InUse:           false,
			MountPoint:      "mount-666_b",
			SerialId:        "serial-666_b",
		},
	}
	err = st.UpdateMachineBlockDevices(
		c.Context(), machineUUID, nil, updated, nil)
	c.Assert(err, tc.ErrorIsNil)

	result, err = st.BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, updated)
}
