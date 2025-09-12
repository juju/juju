// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}

func (s *serviceSuite) service(c *tc.C) *WatchableService {
	return NewWatchableService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
}

func (s *serviceSuite) TestBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)
	blockDeviceUUID := tc.Must(c, uuid.NewUUID).String()

	bd := map[string]blockdevice.BlockDevice{
		blockDeviceUUID: {
			DeviceName:      "foo",
			DeviceLinks:     []string{"a-link"},
			FilesystemLabel: "label",
			FilesystemUUID:  "device-uuid",
			HardwareId:      "hardware-id",
			WWN:             "wwn",
			BusAddress:      "bus-address",
			SizeMiB:         100,
			FilesystemType:  "ext4",
			InUse:           true,
			MountPoint:      "/path",
			SerialId:        "coco-pops",
		},
	}
	s.state.EXPECT().BlockDevices(gomock.Any(), machineUUID).Return(bd, nil)

	result, err := s.service(c).BlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path",
		SerialId:        "coco-pops",
	}})
}

func (s *serviceSuite) TestAllBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mbd := map[machine.Name][]blockdevice.BlockDevice{
		"666": {{
			DeviceName:      "foo",
			DeviceLinks:     []string{"a-link"},
			FilesystemLabel: "label",
			FilesystemUUID:  "device-uuid",
			HardwareId:      "hardware-id",
			WWN:             "wwn",
			BusAddress:      "bus-address",
			SizeMiB:         100,
			FilesystemType:  "ext4",
			InUse:           true,
			MountPoint:      "/path",
			SerialId:        "coco-pops",
		}},
		"667": {{
			DeviceName: "bar",
		}},
	}
	s.state.EXPECT().MachineBlockDevices(gomock.Any()).Return(mbd, nil)

	result, err := s.service(c).AllBlockDevices(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, mbd)
}

func (s *serviceSuite) TestUpdateDevicesNoExisting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	bd := []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path",
		SerialId:        "coco-pops",
	}}
	s.state.EXPECT().BlockDevices(
		gomock.Any(), machineUUID).Return(nil, nil)

	s.state.EXPECT().UpdateMachineBlockDevices(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[string]blockdevice.BlockDevice,
			updated map[string]blockdevice.BlockDevice,
			removed []string,
		) error {
			c.Check(slices.Collect(maps.Values(added)), tc.DeepEquals, bd)
			c.Check(updated, tc.HasLen, 0)
			c.Check(removed, tc.HasLen, 0)
			return nil
		})

	err := s.service(c).UpdateBlockDevices(c.Context(), machineUUID, bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDevicesExistingUpdated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	existingBd := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName: "foo",
		},
	}
	bd := []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path",
		SerialId:        "coco-pops",
	}}
	s.state.EXPECT().BlockDevices(
		gomock.Any(), machineUUID).Return(existingBd, nil)

	s.state.EXPECT().UpdateMachineBlockDevices(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[string]blockdevice.BlockDevice,
			updated map[string]blockdevice.BlockDevice,
			removed []string,
		) error {
			c.Check(added, tc.HasLen, 0)
			c.Check(updated, tc.DeepEquals, map[string]blockdevice.BlockDevice{
				"a": bd[0],
			})
			c.Check(removed, tc.HasLen, 0)
			return nil
		})

	err := s.service(c).UpdateBlockDevices(c.Context(), machineUUID, bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDevicesExistingRemoved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	existingBd := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName: "bar",
		},
	}
	bd := []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path",
		SerialId:        "coco-pops",
	}}
	s.state.EXPECT().BlockDevices(
		gomock.Any(), machineUUID).Return(existingBd, nil)

	s.state.EXPECT().UpdateMachineBlockDevices(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[string]blockdevice.BlockDevice,
			updated map[string]blockdevice.BlockDevice,
			removed []string,
		) error {
			c.Check(slices.Collect(maps.Values(added)), tc.DeepEquals, bd)
			c.Check(updated, tc.HasLen, 0)
			c.Check(removed, tc.DeepEquals, []string{"a"})
			return nil
		})

	err := s.service(c).UpdateBlockDevices(c.Context(), machineUUID, bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)
	s.state.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("666")).Return(machineUUID, nil)

	existingBd := map[string]blockdevice.BlockDevice{
		"a": {
			DeviceName: "bar",
		},
	}
	bd := []blockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path",
		SerialId:        "coco-pops",
	}}
	s.state.EXPECT().BlockDevices(
		gomock.Any(), machineUUID).Return(existingBd, nil)

	s.state.EXPECT().UpdateMachineBlockDevices(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[string]blockdevice.BlockDevice,
			updated map[string]blockdevice.BlockDevice,
			removed []string,
		) error {
			c.Check(slices.Collect(maps.Values(added)), tc.DeepEquals, bd)
			c.Check(updated, tc.HasLen, 0)
			c.Check(removed, tc.DeepEquals, []string{"a"})
			return nil
		})

	err := s.service(c).SetBlockDevices(c.Context(), "666", bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestWatchBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	ch := make(chan struct{})
	close(ch)
	tw := watchertest.NewMockNotifyWatcher(ch)
	defer watchertest.CleanKill(c, tw)

	s.state.EXPECT().NamespaceForWatchBlockDevices().Return("yo")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(), gomock.Any(), gomock.Any()).Return(tw, nil)

	w, err := s.service(c).WatchBlockDevices(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
}
