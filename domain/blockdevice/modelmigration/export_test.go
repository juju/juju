// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	dst.AddMachine(description.MachineArgs{
		Id: "666",
	})
	m := dst.Machines()
	c.Assert(m, tc.HasLen, 1)
	c.Assert(m[0].BlockDevices(), tc.HasLen, 0)

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
	c.Assert(m, tc.HasLen, 1)
	c.Assert(m[0].BlockDevices(), tc.HasLen, 1)
	bd := m[0].BlockDevices()[0]
	c.Check(bd.Name(), tc.Equals, "foo")
	c.Check(bd.Links(), jc.DeepEquals, []string{"a-link"})
	c.Check(bd.Label(), tc.Equals, "label")
	c.Check(bd.UUID(), tc.Equals, "device-uuid")
	c.Check(bd.HardwareID(), tc.Equals, "hardware-id")
	c.Check(bd.WWN(), tc.Equals, "wwn")
	c.Check(bd.BusAddress(), tc.Equals, "bus-address")
	c.Check(bd.SerialID(), tc.Equals, "serial-id")
	c.Check(bd.Size(), tc.Equals, uint64(100))
	c.Check(bd.FilesystemType(), tc.Equals, "ext4")
	c.Check(bd.InUse(), jc.IsTrue)
	c.Check(bd.MountPoint(), tc.Equals, "/path/to/here")
}

func (s *exportSuite) TestExportMachineNotFound(c *tc.C) {
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
