// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type volumeSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) expectedVolumeDetails() params.VolumeDetails {
	return params.VolumeDetails{
		VolumeTag: s.volumeTag.String(),
		Life:      "alive",
		Status: params.EntityStatus{
			Status: "attached",
		},
		MachineAttachments: map[string]params.VolumeAttachmentDetails{
			s.machineTag.String(): {
				Life: "alive",
			},
		},
		UnitAttachments: map[string]params.VolumeAttachmentDetails{},
		Storage: &params.StorageDetails{
			StorageTag: "storage-data-0",
			OwnerTag:   "unit-mysql-0",
			Kind:       params.StorageKindFilesystem,
			Life:       "dying",
			Status: params.EntityStatus{
				Status: "attached",
			},
			Attachments: map[string]params.StorageAttachmentDetails{
				"unit-mysql-0": {
					StorageTag: "storage-data-0",
					UnitTag:    "unit-mysql-0",
					MachineTag: "machine-66",
					Life:       "alive",
				},
			},
		},
	}
}

func (s *volumeSuite) TestListVolumesNoFilters(c *gc.C) {
	defer s.setupMocks(c).Finish()

	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 0)
}

func (s *volumeSuite) TestListVolumesEmptyFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], gc.DeepEquals, s.expectedVolumeDetails())
}

func (s *volumeSuite) TestListVolumesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	msg := "inventing error"
	s.storageAccessor.allVolumes = func() ([]state.Volume, error) {
		return nil, errors.New(msg)
	}
	results, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, msg)
}

func (s *volumeSuite) TestListVolumesNoVolumes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageAccessor.allVolumes = func() ([]state.Volume, error) {
		return nil, nil
	}
	results, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.HasLen, 0)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *volumeSuite) TestListVolumesFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	filters := []params.VolumeFilter{{
		Machines: []string{s.machineTag.String()},
	}}
	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{filters})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, s.expectedVolumeDetails())
}

func (s *volumeSuite) TestListVolumesFilterNonMatching(c *gc.C) {
	defer s.setupMocks(c).Finish()

	filters := []params.VolumeFilter{{
		Machines: []string{"machine-42"},
	}}
	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{filters})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 0)
	c.Assert(found.Results[0].Error, gc.IsNil)
}

func (s *volumeSuite) TestListVolumesVolumeInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.volume.info = &state.VolumeInfo{
		Size:       123,
		HardwareId: "abc",
		Persistent: true,
	}
	expected := s.expectedVolumeDetails()
	expected.Info.Size = 123
	expected.Info.HardwareId = "abc"
	expected.Info.Persistent = true
	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesAttachmentInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.volumeAttachment.info = &state.VolumeAttachmentInfo{
		DeviceName: "xvdf1",
		ReadOnly:   true,
	}
	expected := s.expectedVolumeDetails()
	expected.MachineAttachments[s.machineTag.String()] = params.VolumeAttachmentDetails{
		VolumeAttachmentInfo: params.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
			ReadOnly:   true,
		},
		Life: "alive",
	}
	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesStorageLocationNoBlockDevice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageInstance.kind = state.StorageKindBlock
	s.volume.info = &state.VolumeInfo{}
	s.volumeAttachment.info = &state.VolumeAttachmentInfo{
		ReadOnly: true,
	}
	expected := s.expectedVolumeDetails()
	expected.Storage.Kind = params.StorageKindBlock
	expected.Storage.Status.Status = status.Attached
	expected.MachineAttachments[s.machineTag.String()] = params.VolumeAttachmentDetails{
		VolumeAttachmentInfo: params.VolumeAttachmentInfo{
			ReadOnly: true,
		},
		Life: "alive",
	}
	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesStorageLocationBlockDevicePath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.blockDeviceGetter.blockDevices = func(machineId string) ([]blockdevice.BlockDevice, error) {
		return []blockdevice.BlockDevice{{
			BusAddress: "bus-addr",
			DeviceName: "sdd",
		}}, nil
	}
	s.storageInstance.kind = state.StorageKindBlock
	s.volume.info = &state.VolumeInfo{}
	s.volumeAttachment.info = &state.VolumeAttachmentInfo{
		BusAddress: "bus-addr",
		ReadOnly:   true,
	}
	expected := s.expectedVolumeDetails()
	expected.Storage.Kind = params.StorageKindBlock
	expected.Storage.Status.Status = status.Attached
	storageAttachmentDetails := expected.Storage.Attachments["unit-mysql-0"]
	storageAttachmentDetails.Location = "/dev/sdd"
	expected.Storage.Attachments["unit-mysql-0"] = storageAttachmentDetails
	expected.MachineAttachments[s.machineTag.String()] = params.VolumeAttachmentDetails{
		VolumeAttachmentInfo: params.VolumeAttachmentInfo{
			BusAddress: "bus-addr",
			ReadOnly:   true,
		},
		Life: "alive",
	}
	found, err := s.api.ListVolumes(context.Background(), params.VolumeFilters{[]params.VolumeFilter{{}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}
