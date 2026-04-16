// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/blockdevice"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

type blockDevicesSuite struct{}

func TestBlockDevicesSuite(t *testing.T) {
	tc.Run(t, &blockDevicesSuite{})
}

func minimalDeps(c *tc.C, accessor VolumeAccessor) *dependencies {
	return &dependencies{
		config: Config{
			Scope:   names.NewMachineTag("0"),
			Volumes: accessor,
			Logger:  loggertesting.WrapCheckLog(c),
		},
		volumeBlockDevices: make(
			map[names.VolumeTag]blockdevice.BlockDevice,
		),
		incompleteFilesystemParams: make(
			map[names.FilesystemTag]storage.FilesystemParams,
		),
		incompleteFilesystemAttachmentParams: make(
			map[params.MachineStorageId]storage.FilesystemAttachmentParams,
		),
		filesystems: make(map[names.FilesystemTag]storage.Filesystem),
	}
}

func (s *blockDevicesSuite) TestRefreshVolumeBlockDevicesIsCodeNotProvisioned(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	volumeTag := names.NewVolumeTag("0")
	accessor := NewMockVolumeAccessor(ctrl)
	accessor.EXPECT().VolumeBlockDevices(gomock.Any(), gomock.Any()).Return(
		[]params.BlockDeviceResult{{
			Error: &params.Error{
				Code:    params.CodeNotProvisioned,
				Message: "volume not provisioned",
			},
		}},
		nil,
	)

	deps := minimalDeps(c, accessor)
	updated, err := refreshVolumeBlockDevices(
		c.Context(), deps, []names.VolumeTag{volumeTag},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(updated, tc.IsNil)
	// Block device must not have been recorded; caller should wait
	// for the block device watcher to notify again.
	c.Check(deps.volumeBlockDevices, tc.HasLen, 0)
}

func (s *blockDevicesSuite) TestRefreshVolumeBlockDevicesIsCodeNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	volumeTag := names.NewVolumeTag("1")
	accessor := NewMockVolumeAccessor(ctrl)
	accessor.EXPECT().VolumeBlockDevices(gomock.Any(), gomock.Any()).Return(
		[]params.BlockDeviceResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "block device not found",
			},
		}},
		nil,
	)

	deps := minimalDeps(c, accessor)
	updated, err := refreshVolumeBlockDevices(
		c.Context(), deps, []names.VolumeTag{volumeTag},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(updated, tc.IsNil)
	// Block device must not have been recorded; caller should wait
	// for the block device watcher to notify again.
	c.Check(deps.volumeBlockDevices, tc.HasLen, 0)
}

func (s *blockDevicesSuite) TestRefreshVolumeBlockDevicesOtherErrorPropagates(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	volumeTag := names.NewVolumeTag("2")
	accessor := NewMockVolumeAccessor(ctrl)
	accessor.EXPECT().VolumeBlockDevices(gomock.Any(), gomock.Any()).Return(
		[]params.BlockDeviceResult{{
			Error: &params.Error{
				Code:    params.CodeNotAssigned,
				Message: "volume not assigned",
			},
		}},
		nil,
	)

	deps := minimalDeps(c, accessor)
	_, err := refreshVolumeBlockDevices(
		c.Context(), deps, []names.VolumeTag{volumeTag},
	)
	c.Assert(
		err,
		tc.ErrorMatches,
		`getting block device info for volume attachment .*: volume not assigned`,
	)
}
