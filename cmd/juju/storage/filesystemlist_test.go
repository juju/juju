// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"encoding/json"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/testing"
)

type filesystemListSuite struct {
	SubStorageSuite
	mockAPI *mockFilesystemListAPI
}

var _ = gc.Suite(&filesystemListSuite{})

func (s *filesystemListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockFilesystemListAPI{}
}

func (s *filesystemListSuite) TestFilesystemListEmpty(c *gc.C) {
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		return nil, nil
	}
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		"",
	)
}

func (s *filesystemListSuite) TestFilesystemListError(c *gc.C) {
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		return nil, errors.New("just my luck")
	}
	context, err := s.runFilesystemList(c, "--format", "yaml")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "just my luck")
	s.assertUserFacingOutput(c, context, "", "")
}

func (s *filesystemListSuite) TestFilesystemListArgs(c *gc.C) {
	var called bool
	expectedArgs := []string{"a", "b", "c"}
	s.mockAPI.listFilesystems = func(arg []string) ([]params.FilesystemDetailsListResult, error) {
		c.Assert(arg, jc.DeepEquals, expectedArgs)
		called = true
		return nil, nil
	}
	s.assertValidList(
		c,
		append([]string{"--format", "yaml"}, expectedArgs...),
		"",
	)
	c.Assert(called, jc.IsTrue)
}

func (s *filesystemListSuite) TestFilesystemListYaml(c *gc.C) {
	s.assertUnmarshalledOutput(
		c,
		goyaml.Unmarshal,
		"", // no error
		"--format", "yaml")
}

func (s *filesystemListSuite) TestFilesystemListJSON(c *gc.C) {
	s.assertUnmarshalledOutput(
		c,
		json.Unmarshal,
		"", // no error
		"--format", "json")
}

func (s *filesystemListSuite) TestFilesystemListWithErrorResults(c *gc.C) {
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		results, _ := mockFilesystemListAPI{}.ListFilesystems(nil)
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
MACHINE  UNIT         STORAGE      ID   VOLUME  PROVIDER-ID                       MOUNTPOINT  SIZE    STATE      MESSAGE
0        abc/0        db-dir/1001  0/0  0/1     provider-supplied-filesystem-0-0  /mnt/fuji   512MiB  attached   
0        transcode/0  shared-fs/0  4            provider-supplied-filesystem-4    /mnt/doom   1.0GiB  attached   
0                                  1            provider-supplied-filesystem-1                2.0GiB  attaching  failed to attach, will retry
1        transcode/1  shared-fs/0  4            provider-supplied-filesystem-4    /mnt/huang  1.0GiB  attached   
1                                  2            provider-supplied-filesystem-2    /mnt/zion   3.0MiB  attached   
1                                  3                                                          42MiB   pending    

`[1:]

func (s *filesystemListSuite) TestFilesystemListTabular(c *gc.C) {
	s.assertValidList(c, []string{}, expectedFilesystemListTabular)

	// Do it again, reversing the results returned by the API.
	// We should get everything sorted in the appropriate order.
	s.mockAPI.listFilesystems = func([]string) ([]params.FilesystemDetailsListResult, error) {
		results, _ := mockFilesystemListAPI{}.ListFilesystems(nil)
		n := len(results)
		for i := 0; i < n/2; i++ {
			results[i], results[n-i-1] = results[n-i-1], results[i]
		}
		return results, nil
	}
	s.assertValidList(c, []string{}, expectedFilesystemListTabular)
}

func (s *filesystemListSuite) assertUnmarshalledOutput(c *gc.C, unmarshal unmarshaller, expectedErr string, args ...string) {
	context, err := s.runFilesystemList(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	var result struct {
		Filesystems map[string]storage.FilesystemInfo
	}
	err = unmarshal([]byte(testing.Stdout(context)), &result)
	c.Assert(err, jc.ErrorIsNil)

	expected := s.expect(c, nil)
	c.Assert(result.Filesystems, jc.DeepEquals, expected)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)
}

// expect returns the FilesystemInfo mapping we should expect to unmarshal
// from rendered YAML or JSON.
func (s *filesystemListSuite) expect(c *gc.C, machines []string) map[string]storage.FilesystemInfo {
	all, err := s.mockAPI.ListFilesystems(machines)
	c.Assert(err, jc.ErrorIsNil)

	var valid []params.FilesystemDetails
	for _, result := range all {
		if result.Error == nil {
			valid = append(valid, result.Result...)
		}
	}
	result, err := storage.ConvertToFilesystemInfo(valid)
	c.Assert(err, jc.ErrorIsNil)
	return result
}

func (s *filesystemListSuite) assertValidList(c *gc.C, args []string, expectedOut string) {
	context, err := s.runFilesystemList(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUserFacingOutput(c, context, expectedOut, "")
}

func (s *filesystemListSuite) runFilesystemList(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c,
		storage.NewFilesystemListCommand(s.mockAPI, s.store),
		args...)
}

func (s *filesystemListSuite) assertUserFacingOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	obtainedOut := testing.Stdout(context)
	c.Assert(obtainedOut, gc.Equals, expectedOut)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)
}

type mockFilesystemListAPI struct {
	listFilesystems func([]string) ([]params.FilesystemDetailsListResult, error)
}

func (s mockFilesystemListAPI) Close() error {
	return nil
}

func (s mockFilesystemListAPI) ListFilesystems(machines []string) ([]params.FilesystemDetailsListResult, error) {
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
			Status: createTestStatus(params.StatusAttached, ""),
			MachineAttachments: map[string]params.FilesystemAttachmentInfo{
				"machine-0": params.FilesystemAttachmentInfo{
					MountPoint: "/mnt/fuji",
				},
			},
			Storage: &params.StorageDetails{
				StorageTag: "storage-db-dir-1001",
				OwnerTag:   "unit-abc-0",
				Kind:       params.StorageKindBlock,
				Status:     createTestStatus(params.StatusAttached, ""),
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-abc-0": params.StorageAttachmentDetails{
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
			Status: createTestStatus(params.StatusAttaching, "failed to attach, will retry"),
			MachineAttachments: map[string]params.FilesystemAttachmentInfo{
				"machine-0": params.FilesystemAttachmentInfo{},
			},
		},
		// filesystem 3 is due to be attached to machine 1, but is not
		// assigned to any storage and has not yet been provisioned.
		{
			FilesystemTag: "filesystem-3",
			Info: params.FilesystemInfo{
				Size: 42,
			},
			Status: createTestStatus(params.StatusPending, ""),
			MachineAttachments: map[string]params.FilesystemAttachmentInfo{
				"machine-1": params.FilesystemAttachmentInfo{},
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
			Status: createTestStatus(params.StatusAttached, ""),
			MachineAttachments: map[string]params.FilesystemAttachmentInfo{
				"machine-1": params.FilesystemAttachmentInfo{
					MountPoint: "/mnt/zion",
				},
			},
		},
		// filesystem 4 is attached to machines 0 and 1, and is assigned
		// to shared storage.
		{
			FilesystemTag: "filesystem-4",
			Info: params.FilesystemInfo{
				FilesystemId: "provider-supplied-filesystem-4",
				Size:         1024,
			},
			Status: createTestStatus(params.StatusAttached, ""),
			MachineAttachments: map[string]params.FilesystemAttachmentInfo{
				"machine-0": params.FilesystemAttachmentInfo{
					MountPoint: "/mnt/doom",
					ReadOnly:   true,
				},
				"machine-1": params.FilesystemAttachmentInfo{
					MountPoint: "/mnt/huang",
					ReadOnly:   true,
				},
			},
			Storage: &params.StorageDetails{
				StorageTag: "storage-shared-fs-0",
				OwnerTag:   "service-transcode",
				Kind:       params.StorageKindBlock,
				Status:     createTestStatus(params.StatusAttached, ""),
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-transcode-0": params.StorageAttachmentDetails{
						StorageTag: "storage-shared-fs-0",
						UnitTag:    "unit-transcode-0",
						MachineTag: "machine-0",
						Location:   "/mnt/bits",
					},
					"unit-transcode-1": params.StorageAttachmentDetails{
						StorageTag: "storage-shared-fs-0",
						UnitTag:    "unit-transcode-1",
						MachineTag: "machine-1",
						Location:   "/mnt/pieces",
					},
				},
			},
		},
	}}}
	return results, nil
}
