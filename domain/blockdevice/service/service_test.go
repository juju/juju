// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}

func (s *serviceSuite) service(c *gc.C) *WatchableService {
	return NewWatchableService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
}

func (s *serviceSuite) TestBlockDevices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	bd := []blockdevice.BlockDevice{{
		DeviceName:     "foo",
		DeviceLinks:    []string{"a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareId:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SizeMiB:        100,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/path",
		SerialId:       "coco-pops",
	}}
	s.state.EXPECT().BlockDevices(gomock.Any(), "666").Return(bd, nil)

	result, err := s.service(c).BlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, bd)
}

func (s *serviceSuite) TestAllBlockDevices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	mbd := []blockdevice.MachineBlockDevice{{
		MachineId: "666",
		BlockDevice: blockdevice.BlockDevice{
			DeviceName:     "foo",
			DeviceLinks:    []string{"a-link"},
			Label:          "label",
			UUID:           "device-uuid",
			HardwareId:     "hardware-id",
			WWN:            "wwn",
			BusAddress:     "bus-address",
			SizeMiB:        100,
			FilesystemType: "ext4",
			InUse:          true,
			MountPoint:     "/path",
			SerialId:       "coco-pops",
		},
	}, {
		MachineId: "667",
		BlockDevice: blockdevice.BlockDevice{
			DeviceName: "bar",
		},
	}}
	s.state.EXPECT().MachineBlockDevices(gomock.Any()).Return(mbd, nil)

	result, err := s.service(c).AllBlockDevices(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[string]blockdevice.BlockDevice{
		"666": mbd[0].BlockDevice,
		"667": mbd[1].BlockDevice,
	})
}

func (s *serviceSuite) TestUpdateDevices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	bd := []blockdevice.BlockDevice{{
		DeviceName:     "foo",
		DeviceLinks:    []string{"a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareId:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SizeMiB:        100,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/path",
		SerialId:       "coco-pops",
	}}
	s.state.EXPECT().SetMachineBlockDevices(gomock.Any(), "666", bd)

	err := s.service(c).UpdateBlockDevices(context.Background(), "666", bd...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDevicesNoFilesystemType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	bd := []blockdevice.BlockDevice{{
		DeviceName:     "foo",
		DeviceLinks:    []string{"a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareId:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SizeMiB:        100,
		FilesystemType: "unspecified",
		InUse:          true,
		MountPoint:     "/path",
		SerialId:       "coco-pops",
	}}
	s.state.EXPECT().SetMachineBlockDevices(gomock.Any(), "666", bd)

	in := bd[0]
	in.FilesystemType = ""
	err := s.service(c).UpdateBlockDevices(context.Background(), "666", in)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchBlockDevice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.state.EXPECT().WatchBlockDevices(gomock.Any(), gomock.Any(), "666").Return(nw, nil)

	w, err := s.service(c).WatchBlockDevices(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}
