// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

func (s *ListSuite) TestVolumeListEmpty(c *tc.C) {
	s.mockAPI.listVolumes = func([]string) ([]params.VolumeDetailsListResult, error) {
		return nil, nil
	}
	s.assertValidVolumeList(
		c,
		[]string{"--format", "yaml"},
		"",
	)
}

func (s *ListSuite) TestVolumeListError(c *tc.C) {
	s.mockAPI.listVolumes = func([]string) ([]params.VolumeDetailsListResult, error) {
		return nil, errors.New("just my luck")
	}
	context, err := s.runVolumeList(c, "--format", "yaml")
	c.Assert(errors.Cause(err), tc.ErrorMatches, "just my luck")
	s.assertUserFacingVolumeOutput(c, context, "", "")
}

func (s *ListSuite) TestVolumeListArgs(c *tc.C) {
	var called bool
	expectedArgs := []string{"a", "b", "c"}
	s.mockAPI.listVolumes = func(arg []string) ([]params.VolumeDetailsListResult, error) {
		c.Assert(arg, tc.DeepEquals, expectedArgs)
		called = true
		return nil, nil
	}
	s.assertValidVolumeList(
		c,
		append([]string{"--format", "yaml"}, expectedArgs...),
		"",
	)
	c.Assert(called, tc.IsTrue)
}

func (s *ListSuite) TestVolumeListYaml(c *tc.C) {
	s.assertUnmarshalledVolumeOutput(
		c,
		goyaml.Unmarshal,
		"", // no error
		"--format", "yaml")
}

func (s *ListSuite) TestVolumeListJSON(c *tc.C) {
	s.assertUnmarshalledVolumeOutput(
		c,
		json.Unmarshal,
		"", // no error
		"--format", "json")
}

func (s *ListSuite) TestVolumeListWithErrorResults(c *tc.C) {
	s.mockAPI.listVolumes = func([]string) ([]params.VolumeDetailsListResult, error) {
		var emptyMockAPI mockListAPI
		results, _ := emptyMockAPI.ListVolumes(c.Context(), nil)
		results = append(results, params.VolumeDetailsListResult{
			Error: &params.Error{Message: "bad"},
		})
		results = append(results, params.VolumeDetailsListResult{
			Error: &params.Error{Message: "ness"},
		})
		return results, nil
	}
	// we should see the error in stderr, but it should not
	// otherwise affect the rendering of valid results.
	s.assertUnmarshalledVolumeOutput(c, json.Unmarshal, "bad\nness\n", "--format", "json")
	s.assertUnmarshalledVolumeOutput(c, goyaml.Unmarshal, "bad\nness\n", "--format", "yaml")
}

var expectedVolumeListTabular = `
Machine  Unit         Storage ID   Volume ID  Provider ID                   Device  Size     State      Message
0        abc/0        db-dir/1001  0/0        provider-supplied-volume-0-0  loop0   512 MiB  attached   
0        transcode/0  shared-fs/0  4          provider-supplied-volume-4    xvdf2   1.0 GiB  attached   
0                                  1          provider-supplied-volume-1            2.0 GiB  attaching  failed to attach, will retry
1        transcode/1  shared-fs/0  4          provider-supplied-volume-4    xvdf3   1.0 GiB  attached   
1                                  2          provider-supplied-volume-2    xvdf1   3.0 MiB  attached   
1                                  3                                                42 MiB   pending    
`[1:]

func (s *ListSuite) TestVolumeListTabular(c *tc.C) {
	s.assertValidVolumeList(c, []string{}, expectedVolumeListTabular)

	// Do it again, reversing the results returned by the API.
	// We should get everything sorted in the appropriate order.
	s.mockAPI.listVolumes = func([]string) ([]params.VolumeDetailsListResult, error) {
		var emptyMockAPI mockListAPI
		results, _ := emptyMockAPI.ListVolumes(c.Context(), nil)
		n := len(results)
		for i := 0; i < n/2; i++ {
			results[i], results[n-i-1] = results[n-i-1], results[i]
		}
		return results, nil
	}
	s.assertValidVolumeList(c, []string{}, expectedVolumeListTabular)
}

var expectedCAASVolumeListTabular = `
Unit     Storage ID   Volume ID  Provider ID                 Size     State     Message
mysql/0  db-dir/1001  0          provider-supplied-volume-0  512 MiB  attached  
`[1:]

func (s *ListSuite) TestCAASVolumeListTabular(c *tc.C) {
	s.assertValidFilesystemList(c, []string{}, expectedFilesystemListTabular)

	// Do it again, reversing the results returned by the API.
	// We should get everything sorted in the appropriate order.
	s.mockAPI.listVolumes = func([]string) ([]params.VolumeDetailsListResult, error) {
		results := []params.VolumeDetailsListResult{{Result: []params.VolumeDetails{
			{
				VolumeTag: "volume-0",
				Info: params.VolumeInfo{
					ProviderId: "provider-supplied-volume-0",
					Size:       512,
				},
				Life:   "alive",
				Status: createTestStatus(status.Attached, "", s.mockAPI.time),
				UnitAttachments: map[string]params.VolumeAttachmentDetails{
					"unit-mysql-0": {
						Life: "alive",
						VolumeAttachmentInfo: params.VolumeAttachmentInfo{
							ReadOnly: true,
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
	s.assertValidVolumeList(c, []string{}, expectedCAASVolumeListTabular)
}

func (s *ListSuite) assertUnmarshalledVolumeOutput(c *tc.C, unmarshal unmarshaller, expectedErr string, args ...string) {
	context, err := s.runVolumeList(c, args...)
	c.Assert(err, tc.ErrorIsNil)

	var result struct {
		Volumes map[string]storage.VolumeInfo
	}
	err = unmarshal([]byte(cmdtesting.Stdout(context)), &result)
	c.Assert(err, tc.ErrorIsNil)

	expected := s.expectVolume(c, nil)
	c.Assert(result.Volumes, tc.DeepEquals, expected)

	obtainedErr := cmdtesting.Stderr(context)
	c.Assert(obtainedErr, tc.Equals, expectedErr)
}

// expect returns the VolumeInfo mapping we should expect to unmarshal
// from rendered YAML or JSON.
func (s *ListSuite) expectVolume(c *tc.C, machines []string) map[string]storage.VolumeInfo {
	all, err := s.mockAPI.ListVolumes(c.Context(), machines)
	c.Assert(err, tc.ErrorIsNil)

	var valid []params.VolumeDetails
	for _, result := range all {
		if result.Error == nil {
			valid = append(valid, result.Result...)
		}
	}
	result, err := storage.ConvertToVolumeInfo(valid)
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *ListSuite) assertValidVolumeList(c *tc.C, args []string, expectedOut string) {
	context, err := s.runVolumeList(c, args...)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUserFacingVolumeOutput(c, context, expectedOut, "")
}

func (s *ListSuite) runVolumeList(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c,
		storage.NewListCommandForTest(s.mockAPI, s.store), append(args, "--volume")...)
}

func (s *ListSuite) assertUserFacingVolumeOutput(c *tc.C, context *cmd.Context, expectedOut, expectedErr string) {
	obtainedOut := cmdtesting.Stdout(context)
	c.Assert(obtainedOut, tc.Equals, expectedOut)

	obtainedErr := cmdtesting.Stderr(context)
	c.Assert(obtainedErr, tc.Equals, expectedErr)
}

func (s *mockListAPI) ListVolumes(ctx context.Context, machines []string) ([]params.VolumeDetailsListResult, error) {
	if s.listVolumes != nil {
		return s.listVolumes(machines)
	}
	results := []params.VolumeDetailsListResult{{Result: []params.VolumeDetails{
		// volume 0/0 is attached to machine 0, assigned to
		// storage db-dir/1001, which is attached to unit
		// abc/0.
		{
			VolumeTag: "volume-0-0",
			Info: params.VolumeInfo{
				ProviderId: "provider-supplied-volume-0-0",
				Pool:       "radiance",
				Size:       512,
			},
			Life:   "alive",
			Status: createTestStatus(status.Attached, "", s.time),
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-0": {
					Life: "alive",
					VolumeAttachmentInfo: params.VolumeAttachmentInfo{
						DeviceName: "loop0",
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
						Location:   "/dev/loop0",
					},
				},
			},
		},
		// volume 1 is attaching to machine 0, but is not assigned
		// to any storage.
		{
			VolumeTag: "volume-1",
			Info: params.VolumeInfo{
				ProviderId: "provider-supplied-volume-1",
				HardwareId: "serial blah blah",
				Persistent: true,
				Size:       2048,
			},
			Status: createTestStatus(status.Attaching, "failed to attach, will retry", s.time),
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-0": {},
			},
		},
		// volume 3 is due to be attached to machine 1, but is not
		// assigned to any storage and has not yet been provisioned.
		{
			VolumeTag: "volume-3",
			Info: params.VolumeInfo{
				Size: 42,
			},
			Status: createTestStatus(status.Pending, "", s.time),
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-1": {},
			},
		},
		// volume 2 is due to be attached to machine 1, but is not
		// assigned to any storage and has not yet been provisioned.
		{
			VolumeTag: "volume-2",
			Info: params.VolumeInfo{
				ProviderId: "provider-supplied-volume-2",
				Size:       3,
			},
			Status: createTestStatus(status.Attached, "", s.time),
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-1": {
					VolumeAttachmentInfo: params.VolumeAttachmentInfo{
						DeviceName: "xvdf1",
					},
				},
			},
		},
		// volume 4 is attached to machines 0 and 1, and is assigned
		// to shared storage.
		{
			VolumeTag: "volume-4",
			Info: params.VolumeInfo{
				ProviderId: "provider-supplied-volume-4",
				Persistent: true,
				Size:       1024,
			},
			Status: createTestStatus(status.Attached, "", s.time),
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-0": {
					VolumeAttachmentInfo: params.VolumeAttachmentInfo{
						DeviceName: "xvdf2",
						ReadOnly:   true,
					},
				},
				"machine-1": {
					VolumeAttachmentInfo: params.VolumeAttachmentInfo{
						DeviceName: "xvdf3",
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

func createTestStatus(testStatus status.Status, message string, since time.Time) params.EntityStatus {
	return params.EntityStatus{
		Status: testStatus,
		Info:   message,
		Since:  &since,
	}
}
