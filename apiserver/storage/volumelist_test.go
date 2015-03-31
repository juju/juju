// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/state"
)

type volumeSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) TestGroupAttachmentsByVolumeEmpty(c *gc.C) {
	c.Assert(storage.GroupAttachmentsByVolume(nil), gc.IsNil)
	c.Assert(storage.GroupAttachmentsByVolume([]state.VolumeAttachment{}), gc.IsNil)
}

func (s *volumeSuite) TestGroupAttachmentsByVolume(c *gc.C) {
	volumeTag1 := names.NewVolumeTag("0")
	volumeTag2 := names.NewVolumeTag("0/1")
	machineTag := names.NewMachineTag("0")
	attachments := []state.VolumeAttachment{
		&mockVolumeAttachment{VolumeTag: volumeTag1, MachineTag: machineTag},
		&mockVolumeAttachment{VolumeTag: volumeTag2, MachineTag: machineTag},
		&mockVolumeAttachment{VolumeTag: volumeTag2, MachineTag: machineTag},
	}
	expected := map[string][]params.VolumeAttachment{
		volumeTag1.String(): {
			storage.ConvertStateVolumeAttachmentToParams(attachments[0])},
		volumeTag2.String(): {
			storage.ConvertStateVolumeAttachmentToParams(attachments[1]),
			storage.ConvertStateVolumeAttachmentToParams(attachments[2]),
		},
	}
	c.Assert(
		storage.GroupAttachmentsByVolume(attachments),
		jc.DeepEquals,
		expected)
}

func (s *volumeSuite) TestCreateVolumeItemInvalidTag(c *gc.C) {
	found := storage.CreateVolumeItem(s.api, "666", nil)
	c.Assert(found.Error, gc.ErrorMatches, ".*not a valid tag.*")
}

func (s *volumeSuite) TestCreateVolumeItemNonexistingVolume(c *gc.C) {
	s.state.volume = func(tag names.VolumeTag) (state.Volume, error) {
		return s.volume, errors.Errorf("not volume for tag %v", tag)
	}
	found := storage.CreateVolumeItem(s.api, names.NewVolumeTag("666").String(), nil)
	c.Assert(found.Error, gc.ErrorMatches, ".*volume for tag.*")
}

func (s *volumeSuite) TestCreateVolumeItemNoUnit(c *gc.C) {
	s.storageInstance.owner = names.NewServiceTag("test-service")
	found := storage.CreateVolumeItem(s.api, s.volumeTag.String(), nil)
	c.Assert(found.Error, gc.IsNil)
	c.Assert(found.Error, gc.IsNil)
	expected, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Volume, gc.DeepEquals, expected)
}

func (s *volumeSuite) TestCreateVolumeItemNoStorageInstance(c *gc.C) {
	s.volume = &mockVolume{tag: s.volumeTag, hasNoStorage: true}
	found := storage.CreateVolumeItem(s.api, s.volumeTag.String(), nil)
	c.Assert(found.Error, gc.IsNil)
	c.Assert(found.Error, gc.IsNil)
	expected, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Volume, gc.DeepEquals, expected)
}

func (s *volumeSuite) TestCreateVolumeItem(c *gc.C) {
	found := storage.CreateVolumeItem(s.api, s.volumeTag.String(), nil)
	c.Assert(found.Error, gc.IsNil)
	c.Assert(found.Error, gc.IsNil)
	expected, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Volume, gc.DeepEquals, expected)
}

func (s *volumeSuite) TestGetVolumeItemsEmpty(c *gc.C) {
	c.Assert(storage.GetVolumeItems(s.api, nil), gc.IsNil)
	c.Assert(storage.GetVolumeItems(s.api, []state.VolumeAttachment{}), gc.IsNil)
}

func (s *volumeSuite) TestGetVolumeItems(c *gc.C) {
	machineTag := names.NewMachineTag("0")
	attachments := []state.VolumeAttachment{
		&mockVolumeAttachment{VolumeTag: s.volumeTag, MachineTag: machineTag},
		&mockVolumeAttachment{VolumeTag: s.volumeTag, MachineTag: machineTag},
	}
	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := []params.VolumeItem{
		params.VolumeItem{
			Volume:      expectedVolume,
			Attachments: storage.ConvertStateVolumeAttachmentsToParams(attachments)},
	}
	c.Assert(
		storage.GetVolumeItems(s.api, attachments),
		jc.DeepEquals,
		expected)
}

func (s *volumeSuite) TestFilterVolumesNoItems(c *gc.C) {
	s.state.machineVolumeAttachments =
		func(machine names.MachineTag) ([]state.VolumeAttachment, error) {
			return nil, nil
		}
	filter := params.VolumeFilter{
		Machines: []string{s.machineTag.String()}}

	c.Assert(storage.FilterVolumes(s.api, filter), gc.IsNil)
}

func (s *volumeSuite) TestFilterVolumesErrorMachineAttachments(c *gc.C) {
	s.state.machineVolumeAttachments =
		func(machine names.MachineTag) ([]state.VolumeAttachment, error) {
			return nil, errors.Errorf("not for machine %v", machine)
		}
	filter := params.VolumeFilter{
		Machines: []string{s.machineTag.String()}}

	found := storage.FilterVolumes(s.api, filter)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Error, gc.ErrorMatches, ".*for machine.*")
}

func (s *volumeSuite) TestFilterVolumes(c *gc.C) {
	filter := params.VolumeFilter{
		Machines: []string{s.machineTag.String()}}

	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.VolumeItem{
		Volume: expectedVolume,
		Attachments: storage.ConvertStateVolumeAttachmentsToParams(
			[]state.VolumeAttachment{s.volumeAttachment},
		),
	}
	found := storage.FilterVolumes(s.api, filter)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0], gc.DeepEquals, expected)
}

func (s *volumeSuite) TestVolumeAttachments(c *gc.C) {
	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.VolumeItem{
		Volume: expectedVolume,
		Attachments: storage.ConvertStateVolumeAttachmentsToParams(
			[]state.VolumeAttachment{s.volumeAttachment},
		),
	}

	found := storage.VolumeAttachments(s.api, []state.Volume{s.volume})
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0], gc.DeepEquals, expected)
}

func (s *volumeSuite) TestVolumeAttachmentsEmpty(c *gc.C) {
	s.state.volumeAttachments =
		func(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
			return nil, nil
		}
	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.VolumeItem{
		Volume: expectedVolume,
	}

	found := storage.VolumeAttachments(s.api, []state.Volume{s.volume})
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0], gc.DeepEquals, expected)
}

func (s *volumeSuite) TestVolumeAttachmentsError(c *gc.C) {
	s.state.volumeAttachments =
		func(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
			return nil, errors.Errorf("not for volume %v", volume)
		}

	found := storage.VolumeAttachments(s.api, []state.Volume{s.volume})
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0].Error, gc.ErrorMatches, ".*for volume.*")
}

func (s *volumeSuite) TestListVolumeAttachmentsEmpty(c *gc.C) {
	s.state.allVolumes =
		func() ([]state.Volume, error) {
			return nil, nil
		}
	items, err := storage.ListVolumeAttachments(s.api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(items, gc.IsNil)
}

func (s *volumeSuite) TestListVolumeAttachmentsError(c *gc.C) {
	msg := "inventing error"
	s.state.allVolumes =
		func() ([]state.Volume, error) {
			return nil, errors.New(msg)
		}
	items, err := storage.ListVolumeAttachments(s.api)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(items, gc.IsNil)
}

func (s *volumeSuite) TestListVolumeAttachments(c *gc.C) {
	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.VolumeItem{
		Volume: expectedVolume,
		Attachments: storage.ConvertStateVolumeAttachmentsToParams(
			[]state.VolumeAttachment{s.volumeAttachment},
		),
	}

	items, err := storage.ListVolumeAttachments(s.api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(items, gc.HasLen, 1)
	c.Assert(items[0], gc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesEmptyFilter(c *gc.C) {
	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.VolumeItem{
		Volume: expectedVolume,
		Attachments: storage.ConvertStateVolumeAttachmentsToParams(
			[]state.VolumeAttachment{s.volumeAttachment},
		),
	}
	found, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], gc.DeepEquals, expected)
}

func (s *volumeSuite) TestListVolumesError(c *gc.C) {
	msg := "inventing error"
	s.state.allVolumes =
		func() ([]state.Volume, error) {
			return nil, errors.New(msg)
		}

	items, err := s.api.ListVolumes(params.VolumeFilter{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(items, gc.DeepEquals, params.VolumeItemsResult{})
}

func (s *volumeSuite) TestListVolumesFilter(c *gc.C) {
	expectedVolume, err := storage.ConvertStateVolumeToParams(s.api, s.volume)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.VolumeItem{
		Volume: expectedVolume,
		Attachments: storage.ConvertStateVolumeAttachmentsToParams(
			[]state.VolumeAttachment{s.volumeAttachment},
		),
	}
	filter := params.VolumeFilter{
		Machines: []string{s.machineTag.String()}}
	found, err := s.api.ListVolumes(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0], gc.DeepEquals, expected)
}
