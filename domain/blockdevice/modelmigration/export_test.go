// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() *exportOperation {
	return &exportOperation{
		service: s.service,
	}
}

func (s *exportSuite) TestExport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	dst.AddMachine(description.MachineArgs{
		Id: "666",
	})
	m := dst.Machines()
	c.Assert(m, gc.HasLen, 1)
	c.Assert(m[0].BlockDevices(), gc.HasLen, 0)

	s.service.EXPECT().AllBlockDevices(gomock.Any()).
		Times(1).
		Return(map[string]blockdevice.BlockDevice{
			"666": {
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
			},
		}, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	m = dst.Machines()
	c.Assert(m, gc.HasLen, 1)
	c.Assert(m[0].BlockDevices(), gc.HasLen, 1)
	bd := m[0].BlockDevices()[0]
	c.Check(bd.Name(), gc.Equals, "foo")
	c.Check(bd.Links(), jc.DeepEquals, []string{"a-link"})
	c.Check(bd.Label(), gc.Equals, "label")
	c.Check(bd.UUID(), gc.Equals, "device-uuid")
	c.Check(bd.HardwareID(), gc.Equals, "hardware-id")
	c.Check(bd.WWN(), gc.Equals, "wwn")
	c.Check(bd.BusAddress(), gc.Equals, "bus-address")
	c.Check(bd.SerialID(), gc.Equals, "serial-id")
	c.Check(bd.Size(), gc.Equals, uint64(100))
	c.Check(bd.FilesystemType(), gc.Equals, "ext4")
	c.Check(bd.InUse(), jc.IsTrue)
	c.Check(bd.MountPoint(), gc.Equals, "/path/to/here")
}

func (s *exportSuite) TestExportMachineNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.service.EXPECT().AllBlockDevices(gomock.Any()).
		Times(1).
		Return(map[string]blockdevice.BlockDevice{
			"666": {DeviceName: "foo"},
		}, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}
