// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: "666",
	})
	// And also a machine with no block devices.
	model.AddMachine(description.MachineArgs{
		Id: "667",
	})
	err := model.AddBlockDevice("666", description.BlockDeviceArgs{
		Name:           "foo",
		Links:          []string{"a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareID:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SerialID:       "serial-id",
		Size:           100,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/path/to/here",
	})
	c.Assert(err, tc.ErrorIsNil)

	expectedBlockDevices := []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SerialId:        "serial-id",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path/to/here",
	}}
	s.service.EXPECT().SetBlockDevicesForMachineByName(
		gomock.Any(), machine.Name("666"), expectedBlockDevices).Return(nil)

	op := s.newImportOperation()
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportVolumeAttachmentPlan(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Add machines
	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: "666",
	})
	model.AddMachine(description.MachineArgs{
		Id: "667",
	})

	// Arrange: Add a block device to one.
	err := model.AddBlockDevice("666", description.BlockDeviceArgs{
		Name:           "foo",
		Links:          []string{"/dev/disk/by-id/a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareID:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SerialID:       "serial-id",
		Size:           100,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/path/to/here",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Arrange: Add a volume with an attachment block device matching
	// the machine's above, and with a attachment plan which does not.
	vol := model.AddVolume(description.VolumeArgs{})
	vol.AddAttachment(description.VolumeAttachmentArgs{
		HostMachine: "666",
		BusAddress:  "bus-address",
		DeviceLink:  "/dev/disk/by-id/a-link",
		DeviceName:  "foo",
	})
	vol.AddAttachmentPlan(description.VolumeAttachmentPlanArgs{
		Machine:     "666",
		DeviceName:  "baz",
		DeviceLinks: []string{"/dev/disk/by-id/d-link"},
	})

	// Arrange: expected mock call. Where a block device is found in all
	// three locations, the order of preference is: the machine's block
	// device, the volume attachment plan's block device, lastly the
	// volume attachment's block device.
	expectedBlockDevices := []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"/dev/disk/by-id/a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SerialId:        "serial-id",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path/to/here",
	}, {
		DeviceName:  "baz",
		DeviceLinks: []string{"/dev/disk/by-id/d-link"},
	}}
	s.service.EXPECT().SetBlockDevicesForMachineByName(
		gomock.Any(), machine.Name("666"), expectedBlockDevices).Return(nil)

	// Act
	op := s.newImportOperation()

	// Assert
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
