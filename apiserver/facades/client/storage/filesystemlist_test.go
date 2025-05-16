// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type filesystemSuite struct {
	baseStorageSuite
}

func TestFilesystemSuite(t *stdtesting.T) { tc.Run(t, &filesystemSuite{}) }
func (s *filesystemSuite) expectedFilesystemDetails() params.FilesystemDetails {
	return params.FilesystemDetails{
		FilesystemTag: s.filesystemTag.String(),
		Life:          "alive",
		Status: params.EntityStatus{
			Status: "attached",
		},
		MachineAttachments: map[string]params.FilesystemAttachmentDetails{
			s.machineTag.String(): {
				Life: "dead",
			},
		},
		UnitAttachments: map[string]params.FilesystemAttachmentDetails{},
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

func (s *filesystemSuite) TestListFilesystemsEmptyFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	found, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, s.expectedFilesystemDetails())
}

func (s *filesystemSuite) TestListFilesystemsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	msg := "inventing error"
	s.storageAccessor.allFilesystems = func() ([]state.Filesystem, error) {
		return nil, errors.New(msg)
	}
	results, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, msg)
}

func (s *filesystemSuite) TestListFilesystemsNoFilesystems(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageAccessor.allFilesystems = func() ([]state.Filesystem, error) {
		return nil, nil
	}
	results, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *filesystemSuite) TestListFilesystemsFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	filters := []params.FilesystemFilter{{
		Machines: []string{s.machineTag.String()},
	}}
	found, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{filters})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, s.expectedFilesystemDetails())
}

func (s *filesystemSuite) TestListFilesystemsFilterNonMatching(c *tc.C) {
	defer s.setupMocks(c).Finish()

	filters := []params.FilesystemFilter{{
		Machines: []string{"machine-42"},
	}}
	found, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{filters})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Error, tc.IsNil)
	c.Assert(found.Results[0].Result, tc.HasLen, 0)
}

func (s *filesystemSuite) TestListFilesystemsFilesystemInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.filesystem.info = &state.FilesystemInfo{
		Size: 123,
	}
	expected := s.expectedFilesystemDetails()
	expected.Info.Size = 123
	found, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, expected)
}

func (s *filesystemSuite) TestListFilesystemsAttachmentInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.filesystemAttachment.info = &state.FilesystemAttachmentInfo{
		MountPoint: "/tmp",
		ReadOnly:   true,
	}
	expected := s.expectedFilesystemDetails()
	expected.MachineAttachments[s.machineTag.String()] = params.FilesystemAttachmentDetails{
		FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
			MountPoint: "/tmp",
			ReadOnly:   true,
		},
		Life: "dead",
	}
	expectedStorageAttachmentDetails := expected.Storage.Attachments["unit-mysql-0"]
	expectedStorageAttachmentDetails.Location = "/tmp"
	expected.Storage.Attachments["unit-mysql-0"] = expectedStorageAttachmentDetails
	found, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, expected)
}

func (s *filesystemSuite) TestListFilesystemsVolumeBacked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.filesystem.volume = &s.volumeTag
	expected := s.expectedFilesystemDetails()
	expected.VolumeTag = s.volumeTag.String()
	found, err := s.api.ListFilesystems(c.Context(), params.FilesystemFilters{
		[]params.FilesystemFilter{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Results, tc.HasLen, 1)
	c.Assert(found.Results[0].Result, tc.HasLen, 1)
	c.Assert(found.Results[0].Result[0], tc.DeepEquals, expected)
}
