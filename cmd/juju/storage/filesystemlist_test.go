// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

func (s *ListSuite) TestFilesystemListEmpty(c *tc.C) {
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		return nil, nil
	}
	s.assertValidFilesystemList(
		c,
		[]string{"--format", "yaml"},
		"",
	)
}

func (s *ListSuite) TestFilesystemListError(c *tc.C) {
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		return nil, errors.New("just my luck")
	}
	context, err := s.runFilesystemList(c, "--format", "yaml")
	c.Assert(errors.Cause(err), tc.ErrorMatches, "just my luck")
	s.assertUserFacingOutput(c, context, "", "")
}

func (s *ListSuite) TestFilesystemListArgs(c *tc.C) {
	var called bool
	expectedArgs := []string{"a", "b", "c"}
	s.mockAPI.listFilesystems = func(arg []string) ([]params.FilesystemDetailsListResult, error) {
		c.Assert(arg, tc.DeepEquals, expectedArgs)
		called = true
		return nil, nil
	}
	s.assertValidFilesystemList(
		c,
		append([]string{"--format", "yaml"}, expectedArgs...),
		"",
	)
	c.Assert(called, tc.IsTrue)
}

func (s *ListSuite) TestFilesystemListYaml(c *tc.C) {
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		"", // no error
		"--format", "yaml")
}

func (s *ListSuite) TestFilesystemListJSON(c *tc.C) {
	s.assertUnmarshalledOutput(
		c,
		json.Unmarshal,
		"", // no error
		"--format", "json")
}

func (s *ListSuite) TestFilesystemListWithErrorResults(c *tc.C) {
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		var emptyMockAPI mockListAPI
		results, _ := emptyMockAPI.ListFilesystems(c.Context(), nil)
		results = append(results, params.FilesystemDetailsListResult{
			Error: &params.Error{Message: "bad"},
		})
		results = append(results, params.FilesystemDetailsListResult{
			Error: &params.Error{Message: "ness"},
		})
		return results, nil
	}
	// we should see the error in stderr, but it should not
	// otherwise affect the rendering of valid results.
	s.assertUnmarshalledOutput(c, json.Unmarshal, "bad\nness\n", "--format", "json")
	s.assertUnmarshalledOutput(c, goyaml.Unmarshal, "bad\nness\n", "--format", "yaml")
}

var expectedFilesystemListTabular = `
Machine  Unit         Storage ID   ID   Volume  Provider ID                       Mountpoint  Size     State      Message
0        abc/0        db-dir/1001  0/0  0/1     provider-supplied-filesystem-0-0  /mnt/fuji   512 MiB  attached   
0        transcode/0  shared-fs/0  4            provider-supplied-filesystem-4    /mnt/doom   1.0 GiB  attached   
0                                  1            provider-supplied-filesystem-1                2.0 GiB  attaching  failed to attach, will retry
1        transcode/1  shared-fs/0  4            provider-supplied-filesystem-4    /mnt/huang  1.0 GiB  attached   
1                                  2            provider-supplied-filesystem-2    /mnt/zion   3.0 MiB  attached   
1                                  3                                                          42 MiB   pending    
`[1:]

func (s *ListSuite) TestFilesystemListTabular(c *tc.C) {
	s.assertValidFilesystemList(c, []string{}, expectedFilesystemListTabular)

	// Do it again, reversing the results returned by the API.
	// We should get everything sorted in the appropriate order.
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		results, _ := mockListAPI{}.ListFilesystems(c.Context(), nil)
		n := len(results)
		for i := 0; i < n/2; i++ {
			results[i], results[n-i-1] = results[n-i-1], results[i]
		}
		return results, nil
	}
	s.assertValidFilesystemList(c, []string{}, expectedFilesystemListTabular)
}

var expectedCAASFilesystemListTabular = `
Unit     Storage ID   ID   Provider ID                       Mountpoint  Size     State     Message
mysql/0  db-dir/1001  0/0  provider-supplied-filesystem-0-0  /mnt/fuji   512 MiB  attached  
`[1:]

func (s *ListSuite) TestCAASFilesystemListTabular(c *tc.C) {
	s.assertValidFilesystemList(c, []string{}, expectedFilesystemListTabular)

	// Do it again, reversing the results returned by the API.
	// We should get everything sorted in the appropriate order.
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		results := []params.FilesystemDetailsListResult{{Result: []params.FilesystemDetails{
			{
				FilesystemTag: "filesystem-0-0",
				Info: params.FilesystemInfo{
					FilesystemId: "provider-supplied-filesystem-0-0",
					Size:         512,
				},
				Life:   "alive",
				Status: createTestStatus(status.Attached, "", s.mockAPI.time),
				UnitAttachments: map[string]params.FilesystemAttachmentDetails{
					"unit-mysql-0": {
						Life: "alive",
						FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
							MountPoint: "/mnt/fuji",
						},
					},
				},
				Storage: &params.StorageDetails{
					StorageTag: "storage-db-dir-1001",
					OwnerTag:   "unit-abc-0",
					Kind:       params.StorageKindBlock,
					Life:       "alive",
					Status:     createTestStatus(status.Attached, "", s.mockAPI.time),
					Attachments: map[string]params.StorageAttachmentDetails{
						"unit-mysql-0": {
							StorageTag: "storage-db-dir-1001",
							UnitTag:    "unit-abc-0",
							MachineTag: "machine-0",
							Location:   "/mnt/fuji",
						},
					},
				},
			},
		}}}
		return results, nil
	}
	s.assertValidFilesystemList(c, []string{}, expectedCAASFilesystemListTabular)
}

func (s *ListSuite) assertUnmarshalledOutput(c *tc.C, unmarshal unmarshaller, expectedErr string, args ...string) {
	context, err := s.runFilesystemList(c, args...)
	c.Assert(err, tc.ErrorIsNil)

	var result struct {
		Filesystems map[string]storage.FilesystemInfo
	}
	err = unmarshal([]byte(cmdtesting.Stdout(context)), &result)
	c.Assert(err, tc.ErrorIsNil)

	expected := s.expect(c, nil)
	c.Assert(result.Filesystems, tc.DeepEquals, expected)

	obtainedErr := cmdtesting.Stderr(context)
	c.Assert(obtainedErr, tc.Equals, expectedErr)
}

// expect returns the FilesystemInfo mapping we should expect to unmarshal
// from rendered YAML or JSON.
func (s *ListSuite) expect(c *tc.C, machines []string) map[string]storage.FilesystemInfo {
	all, err := s.mockAPI.ListFilesystems(c.Context(), machines)
	c.Assert(err, tc.ErrorIsNil)

	var valid []params.FilesystemDetails
	for _, result := range all {
		if result.Error == nil {
			valid = append(valid, result.Result...)
		}
	}
	result, err := storage.ConvertToFilesystemInfo(valid)
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *ListSuite) assertValidFilesystemList(c *tc.C, args []string, expectedOut string) {
	context, err := s.runFilesystemList(c, args...)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUserFacingOutput(c, context, expectedOut, "")
}

func (s *ListSuite) runFilesystemList(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c,
		storage.NewListCommandForTest(s.mockAPI, s.store), append(args, "--filesystem")...)
}

func (s *ListSuite) assertUserFacingOutput(c *tc.C, context *cmd.Context, expectedOut, expectedErr string) {
	obtainedOut := cmdtesting.Stdout(context)
	c.Assert(obtainedOut, tc.Equals, expectedOut)

	obtainedErr := cmdtesting.Stderr(context)
	c.Assert(obtainedErr, tc.Equals, expectedErr)
}

func (s mockListAPI) ListFilesystems(ctx context.Context, machines []string) ([]params.FilesystemDetailsListResult, error) {
	if s.listFilesystems != nil {
		return s.listFilesystems(machines)
	}
	results := []params.FilesystemDetailsListResult{{Result: []params.FilesystemDetails{
		// filesystem 0/0 is attached to machine 0, assigned to
		// storage db-dir/1001, which is attached to unit
		// abc/0.
		{
			FilesystemTag: "filesystem-0-0",
			VolumeTag:     "volume-0-1",
			Info: params.FilesystemInfo{
				FilesystemId: "provider-supplied-filesystem-0-0",
				Size:         512,
			},
			Life:   "alive",
			Status: createTestStatus(status.Attached, "", s.time),
			MachineAttachments: map[string]params.FilesystemAttachmentDetails{
				"machine-0": {
					Life: "alive",
					FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
						MountPoint: "/mnt/fuji",
					},
				},
			},
			Storage: &params.StorageDetails{
				StorageTag: "storage-db-dir-1001",
				OwnerTag:   "unit-abc-0",
				Kind:       params.StorageKindBlock,
				Life:       "alive",
				Status:     createTestStatus(status.Attached, "", s.time),
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-abc-0": {
						StorageTag: "storage-db-dir-1001",
						UnitTag:    "unit-abc-0",
						MachineTag: "machine-0",
						Location:   "/mnt/fuji",
					},
				},
			},
		},
		// filesystem 1 is attaching to machine 0, but is not assigned
		// to any storage.
		{
			FilesystemTag: "filesystem-1",
			Info: params.FilesystemInfo{
				FilesystemId: "provider-supplied-filesystem-1",
				Size:         2048,
			},
			Status: createTestStatus(status.Attaching, "failed to attach, will retry", s.time),
			MachineAttachments: map[string]params.FilesystemAttachmentDetails{
				"machine-0": {},
			},
		},
		// filesystem 3 is due to be attached to machine 1, but is not
		// assigned to any storage and has not yet been provisioned.
		{
			FilesystemTag: "filesystem-3",
			Info: params.FilesystemInfo{
				Size: 42,
			},
			Status: createTestStatus(status.Pending, "", s.time),
			MachineAttachments: map[string]params.FilesystemAttachmentDetails{
				"machine-1": {},
			},
		},
		// filesystem 2 is due to be attached to machine 1, but is not
		// assigned to any storage.
		{
			FilesystemTag: "filesystem-2",
			Info: params.FilesystemInfo{
				FilesystemId: "provider-supplied-filesystem-2",
				Size:         3,
			},
			Status: createTestStatus(status.Attached, "", s.time),
			MachineAttachments: map[string]params.FilesystemAttachmentDetails{
				"machine-1": {
					FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
						MountPoint: "/mnt/zion",
					},
				},
			},
		},
		// filesystem 4 is attached to machines 0 and 1, and is assigned
		// to shared storage.
		{
			FilesystemTag: "filesystem-4",
			Info: params.FilesystemInfo{
				FilesystemId: "provider-supplied-filesystem-4",
				Pool:         "radiance",
				Size:         1024,
			},
			Status: createTestStatus(status.Attached, "", s.time),
			MachineAttachments: map[string]params.FilesystemAttachmentDetails{
				"machine-0": {
					FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
						MountPoint: "/mnt/doom",
						ReadOnly:   true,
					},
				},
				"machine-1": {
					FilesystemAttachmentInfo: params.FilesystemAttachmentInfo{
						MountPoint: "/mnt/huang",
						ReadOnly:   true,
					},
				},
			},
			Storage: &params.StorageDetails{
				StorageTag: "storage-shared-fs-0",
				OwnerTag:   "application-transcode",
				Kind:       params.StorageKindBlock,
				Status:     createTestStatus(status.Attached, "", s.time),
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-transcode-0": {
						StorageTag: "storage-shared-fs-0",
						UnitTag:    "unit-transcode-0",
						MachineTag: "machine-0",
						Location:   "/mnt/bits",
					},
					"unit-transcode-1": {
						StorageTag: "storage-shared-fs-0",
						UnitTag:    "unit-transcode-1",
						MachineTag: "machine-1",
						Location:   "/mnt/pieces",
					},
				},
			},
		}, {
			// filesystem 5 is assigned to db-dir/1100, but is not yet
			// attached to any machines.
			FilesystemTag: "filesystem-5",
			Info: params.FilesystemInfo{
				FilesystemId: "provider-supplied-filesystem-5",
				Size:         3,
			},
			Status: createTestStatus(status.Attached, "", s.time),
			Storage: &params.StorageDetails{
				StorageTag: "storage-db-dir-1100",
				OwnerTag:   "unit-abc-0",
				Kind:       params.StorageKindBlock,
				Life:       "alive",
				Status:     createTestStatus(status.Attached, "", s.time),
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-abc-0": {
						StorageTag: "storage-db-dir-1100",
						UnitTag:    "unit-abc-0",
						Location:   "/mnt/fuji",
					},
				},
			},
		},
	}}}
	if s.omitPool {
		for _, result := range results {
			for i, details := range result.Result {
				details.Info.Pool = ""
				result.Result[i] = details
			}
		}
	}
	return results, nil
}
