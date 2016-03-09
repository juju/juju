// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type filesystemSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&filesystemSuite{})

func (s *filesystemSuite) expectedFilesystemDetails() params.FilesystemDetails {
	return params.FilesystemDetails{
		FilesystemTag: s.filesystemTag.String(),
		Status: params.EntityStatus{
			Status: "attached",
		},
		MachineAttachments: map[string]params.FilesystemAttachmentInfo{
			s.machineTag.String(): params.FilesystemAttachmentInfo{},
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
	}
}

func (s *filesystemSuite) TestListFilesystemsEmptyFilter(c *gc.C) {
	found, err := s.api.ListFilesystems(params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], gc.DeepEquals, s.expectedFilesystemDetails())
}

func (s *filesystemSuite) TestListFilesystemsError(c *gc.C) {
	msg := "inventing error"
	s.state.allFilesystems = func() ([]state.Filesystem, error) {
		return nil, errors.New(msg)
	}
	results, err := s.api.ListFilesystems(params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, msg)
}

func (s *filesystemSuite) TestListFilesystemsNoFilesystems(c *gc.C) {
	s.state.allFilesystems = func() ([]state.Filesystem, error) {
		return nil, nil
	}
	results, err := s.api.ListFilesystems(params.FilesystemFilters{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *filesystemSuite) TestListFilesystemsFilter(c *gc.C) {
	filters := []params.FilesystemFilter{{
		Machines: []string{s.machineTag.String()},
	}}
	found, err := s.api.ListFilesystems(params.FilesystemFilters{filters})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, s.expectedFilesystemDetails())
}

func (s *filesystemSuite) TestListFilesystemsFilterNonMatching(c *gc.C) {
	filters := []params.FilesystemFilter{{
		Machines: []string{"machine-42"},
	}}
	found, err := s.api.ListFilesystems(params.FilesystemFilters{filters})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.IsNil)
	c.Assert(found.Results[0].Result, gc.HasLen, 0)
}

func (s *filesystemSuite) TestListFilesystemsFilesystemInfo(c *gc.C) {
	s.filesystem.info = &state.FilesystemInfo{
		Size: 123,
	}
	expected := s.expectedFilesystemDetails()
	expected.Info.Size = 123
	found, err := s.api.ListFilesystems(params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}

func (s *filesystemSuite) TestListFilesystemsAttachmentInfo(c *gc.C) {
	s.filesystemAttachment.info = &state.FilesystemAttachmentInfo{
		MountPoint: "/tmp",
		ReadOnly:   true,
	}
	expected := s.expectedFilesystemDetails()
	expected.MachineAttachments[s.machineTag.String()] = params.FilesystemAttachmentInfo{
		MountPoint: "/tmp",
		ReadOnly:   true,
	}
	expectedStorageAttachmentDetails := expected.Storage.Attachments["unit-mysql-0"]
	expectedStorageAttachmentDetails.Location = "/tmp"
	expected.Storage.Attachments["unit-mysql-0"] = expectedStorageAttachmentDetails
	found, err := s.api.ListFilesystems(params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}

func (s *filesystemSuite) TestListFilesystemsVolumeBacked(c *gc.C) {
	s.filesystem.volume = &s.volumeTag
	expected := s.expectedFilesystemDetails()
	expected.VolumeTag = s.volumeTag.String()
	found, err := s.api.ListFilesystems(params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Result, gc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], jc.DeepEquals, expected)
}
