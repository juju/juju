// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/blockdevice"
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
	// No import executed.
	s.service.EXPECT().UpdateBlockDevices(gomock.All(), gomock.Any(), gomock.Any()).Times(0)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: "666",
	})
	// And also a machine with no block devices.
	model.AddMachine(description.MachineArgs{
		Id: "666",
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
	s.service.EXPECT().UpdateBlockDevices(gomock.Any(), "666", blockdevice.BlockDevice{
		DeviceName:     "foo",
		DeviceLinks:    []string{"a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareId:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SerialId:       "serial-id",
		SizeMiB:        100,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/path/to/here",
	}).Times(1)

	op := s.newImportOperation()
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
