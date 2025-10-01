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

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
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

func (s *serviceSuite) TestListBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bdUUID1 := tc.Must(c, blockdevice.NewBlockDeviceUUID)
	bdUUID2 := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	bdDetails := []blockdevice.BlockDeviceDetails{
		{
			UUID:             bdUUID1.String(),
			BlockDeviceName:  "foo",
			BlockDeviceLinks: []string{"a-link"},
			HardwareID:       "hardware-id",
			WWN:              "wwn",
		},
		{
			UUID:            bdUUID2.String(),
			BlockDeviceName: "bar",
			HardwareID:      "hardware-id-2",
			WWN:             "wwn-2",
		},
	}
	s.state.EXPECT().ListBlockDevices(
		gomock.Any(), bdUUID1.String(), bdUUID2.String()).Return(bdDetails, nil)

	result, err := s.service(c).ListBlockDevices(
		c.Context(), bdUUID1, bdUUID2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, bdDetails)
}

func (s *serviceSuite) TestGetBlockDevice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	blockDeviceUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	bd := coreblockdevice.BlockDevice{
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
	}
	s.state.EXPECT().GetBlockDevice(
		gomock.Any(), blockDeviceUUID).Return(bd, nil)

	result, err := s.service(c).GetBlockDevice(
		c.Context(), blockDeviceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coreblockdevice.BlockDevice{
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
	})
}

func (s *serviceSuite) TestGetBlockDeviceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	blockDeviceUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	s.state.EXPECT().GetBlockDevice(gomock.Any(), blockDeviceUUID).Return(
		coreblockdevice.BlockDevice{}, blockdeviceerrors.BlockDeviceNotFound)

	_, err := s.service(c).GetBlockDevice(
		c.Context(), blockDeviceUUID)
	c.Assert(err, tc.ErrorIs, blockdeviceerrors.BlockDeviceNotFound)
}

func (s *serviceSuite) TestGetBlockDeviceInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	blockDeviceUUID := blockdevice.BlockDeviceUUID("foo")

	_, err := s.service(c).GetBlockDevice(
		c.Context(), blockDeviceUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)
	blockDeviceUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)

	bd := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
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
	s.state.EXPECT().GetBlockDevicesForMachine(
		gomock.Any(), machineUUID).Return(bd, nil)

	result, err := s.service(c).GetBlockDevicesForMachine(
		c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []coreblockdevice.BlockDevice{{
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

	mbd := map[machine.Name][]coreblockdevice.BlockDevice{
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
	s.state.EXPECT().GetBlockDevicesForAllMachines(
		gomock.Any()).Return(mbd, nil)

	result, err := s.service(c).GetBlockDevicesForAllMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, mbd)
}

func (s *serviceSuite) TestUpdateDevicesNoExisting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	bd := []coreblockdevice.BlockDevice{{
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
	s.state.EXPECT().GetBlockDevicesForMachine(
		gomock.Any(), machineUUID).Return(nil, nil)

	s.state.EXPECT().UpdateBlockDevicesForMachine(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			updated map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			removed []blockdevice.BlockDeviceUUID,
		) error {
			c.Check(slices.Collect(maps.Values(added)), tc.DeepEquals, bd)
			c.Check(updated, tc.HasLen, 0)
			c.Check(removed, tc.HasLen, 0)
			return nil
		})

	err := s.service(c).UpdateBlockDevicesForMachine(
		c.Context(), machineUUID, bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDevicesExistingUpdated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	existingBd := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		"a": {
			DeviceName: "foo",
		},
	}
	bd := []coreblockdevice.BlockDevice{{
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
	s.state.EXPECT().GetBlockDevicesForMachine(
		gomock.Any(), machineUUID).Return(existingBd, nil)

	s.state.EXPECT().UpdateBlockDevicesForMachine(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			updated map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			removed []blockdevice.BlockDeviceUUID,
		) error {
			c.Check(added, tc.HasLen, 0)
			c.Check(updated, tc.DeepEquals,
				map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
					"a": bd[0],
				},
			)
			c.Check(removed, tc.HasLen, 0)
			return nil
		})

	err := s.service(c).UpdateBlockDevicesForMachine(c.Context(), machineUUID, bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDevicesExistingRemoved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)

	existingBd := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		"a": {
			DeviceName: "bar",
		},
	}
	bd := []coreblockdevice.BlockDevice{{
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
	s.state.EXPECT().GetBlockDevicesForMachine(
		gomock.Any(), machineUUID).Return(existingBd, nil)

	s.state.EXPECT().UpdateBlockDevicesForMachine(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			updated map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			removed []blockdevice.BlockDeviceUUID,
		) error {
			c.Check(slices.Collect(maps.Values(added)), tc.DeepEquals, bd)
			c.Check(updated, tc.HasLen, 0)
			c.Check(removed, tc.DeepEquals, []blockdevice.BlockDeviceUUID{"a"})
			return nil
		})

	err := s.service(c).UpdateBlockDevicesForMachine(c.Context(), machineUUID, bd)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := tc.Must(c, machine.NewUUID)
	s.state.EXPECT().GetMachineUUIDByName(
		gomock.Any(), machine.Name("666")).Return(machineUUID, nil)

	existingBd := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		"a": {
			DeviceName: "bar",
		},
	}
	bd := []coreblockdevice.BlockDevice{{
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
	s.state.EXPECT().GetBlockDevicesForMachine(
		gomock.Any(), machineUUID).Return(existingBd, nil)

	s.state.EXPECT().UpdateBlockDevicesForMachine(
		gomock.Any(), machineUUID, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, _ machine.UUID,
			added map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			updated map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
			removed []blockdevice.BlockDeviceUUID,
		) error {
			c.Check(slices.Collect(maps.Values(added)), tc.DeepEquals, bd)
			c.Check(updated, tc.HasLen, 0)
			c.Check(removed, tc.DeepEquals, []blockdevice.BlockDeviceUUID{"a"})
			return nil
		})

	err := s.service(c).SetBlockDevicesForMachineByName(c.Context(), "666", bd)
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

	w, err := s.service(c).WatchBlockDevicesForMachine(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
}
