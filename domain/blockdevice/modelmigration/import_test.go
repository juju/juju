// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v6"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator)
}

func (s *importSuite) TestNoBlockDevices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	// No import executed.
	s.service.EXPECT().UpdateBlockDevices(gomock.All(), gomock.Any(), gomock.Any()).Times(0)
}

func (s *importSuite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: names.NewMachineTag("666"),
	})
	// And also a machine with no block devices.
	model.AddMachine(description.MachineArgs{
		Id: names.NewMachineTag("668"),
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
	c.Assert(err, jc.ErrorIsNil)
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
	err = op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
