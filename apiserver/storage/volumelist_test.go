// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type volumeSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) expectedVolumeDetailsResult() params.VolumeDetailsResult {
	return params.VolumeDetailsResult{
		Details: &params.VolumeDetails{
			VolumeTag: s.volumeTag.String(),
			Status: params.EntityStatus{
				Status: "attached",
			},
			MachineAttachments: map[string]params.VolumeAttachmentInfo{
				s.machineTag.String(): params.VolumeAttachmentInfo{},
			},
			Storage: &params.StorageDetails{
				StorageTag: "storage-data-0",
				OwnerTag:   "unit-mysql-0",
				Kind:       params.StorageKindFilesystem,
				Status: params.EntityStatus{
					Status: "attached",
				},
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-mysql-0": params.StorageAttachmentDetails{
						StorageTag: "storage-data-0",
						UnitTag:    "unit-mysql-0",
						MachineTag: "machine-66",
					},
				},
			},
		},
		LegacyVolume: &params.LegacyVolumeDetails{
			VolumeTag:  s.volumeTag.String(),
			StorageTag: "storage-data-0",
			UnitTag:    "unit-mysql-0",
			Status: params.EntityStatus{
				Status: "attached",
			},
		},
		LegacyAttachments: []params.VolumeAttachment{{
			VolumeTag:  s.volumeTag.String(),
			MachineTag: s.machineTag.String(),
		}},
	}
}

func (s *volumeSuite) TestListVolumesEmptyFilter(c *gc.C) {
	found, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], gc.DeepEquals, s.expectedVolumeDetailsResult())
}

func (s *volumeSuite) TestListVolumesError(c *gc.C) {
	msg := "inventing error"
	s.state.allVolumes = func() ([]state.Volume, error) {
		return nil, errors.New(msg)
	}
	_, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *volumeSuite) TestListVolumesNoVolumes(c *gc.C) {
	s.state.allVolumes = func() ([]state.Volume, error) {
		return nil, nil
	}
	results, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *volumeSuite) TestListVolumesFilter(c *gc.C) {
	filter := params.VolumeFilter{
		Machines: []string{s.machineTag.String()},
	}
	found, err := s.api.ListVolumes(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], jc.DeepEquals, s.expectedVolumeDetailsResult())
}

func (s *volumeSuite) TestListVolumesFilterNonMatching(c *gc.C) {
	filter := params.VolumeFilter{
		Machines: []string{"machine-42"},
	}
	found, err := s.api.ListVolumes(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 0)
}

func (s *volumeSuite) TestListVolumesVolumeInfo(c *gc.C) {
	s.volume.info = &state.VolumeInfo{
		Size:       123,
		HardwareId: "abc",
		Persistent: true,
	}
	expected := s.expectedVolumeDetailsResult()
	expected.Details.Info.Size = 123
	expected.Details.Info.HardwareId = "abc"
	expected.Details.Info.Persistent = true
	expected.LegacyVolume.Size = 123
	expected.LegacyVolume.HardwareId = "abc"
	expected.LegacyVolume.Persistent = true
	found, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], jc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesAttachmentInfo(c *gc.C) {
	s.volumeAttachment.info = &state.VolumeAttachmentInfo{
		DeviceName: "xvdf1",
		ReadOnly:   true,
	}
	expected := s.expectedVolumeDetailsResult()
	expected.Details.MachineAttachments[s.machineTag.String()] = params.VolumeAttachmentInfo{
		DeviceName: "xvdf1",
		ReadOnly:   true,
	}
	expected.LegacyAttachments[0].Info = params.VolumeAttachmentInfo{
		DeviceName: "xvdf1",
		ReadOnly:   true,
	}
	found, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], jc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesStorageLocationNoBlockDevice(c *gc.C) {
	s.storageInstance.kind = state.StorageKindBlock
	s.volume.info = &state.VolumeInfo{}
	s.volumeAttachment.info = &state.VolumeAttachmentInfo{
		ReadOnly: true,
	}
	expected := s.expectedVolumeDetailsResult()
	expected.Details.Storage.Kind = params.StorageKindBlock
	expected.Details.Storage.Status.Status = params.StatusAttached
	expected.Details.MachineAttachments[s.machineTag.String()] = params.VolumeAttachmentInfo{
		ReadOnly: true,
	}
	expected.LegacyAttachments[0].Info = params.VolumeAttachmentInfo{
		ReadOnly: true,
	}
	found, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], jc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesStorageLocationBlockDevicePath(c *gc.C) {
	s.state.blockDevices = func(names.MachineTag) ([]state.BlockDeviceInfo, error) {
		return []state.BlockDeviceInfo{{
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
	expected := s.expectedVolumeDetailsResult()
	expected.Details.Storage.Kind = params.StorageKindBlock
	expected.Details.Storage.Status.Status = params.StatusAttached
	storageAttachmentDetails := expected.Details.Storage.Attachments["unit-mysql-0"]
	storageAttachmentDetails.Location = filepath.FromSlash("/dev/sdd")
	expected.Details.Storage.Attachments["unit-mysql-0"] = storageAttachmentDetails
	expected.Details.MachineAttachments[s.machineTag.String()] = params.VolumeAttachmentInfo{
		BusAddress: "bus-addr",
		ReadOnly:   true,
	}
	expected.LegacyAttachments[0].Info = params.VolumeAttachmentInfo{
		BusAddress: "bus-addr",
		ReadOnly:   true,
	}
	found, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], jc.DeepEquals, expected)
}
