// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/cmd/juju/status"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type MinimalStatusSuite struct {
	testing.BaseSuite

	statusapi *fakeStatusAPI
	clock     *timeRecorder
}

var _ = gc.Suite(&MinimalStatusSuite{})

func (s *MinimalStatusSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.statusapi = &fakeStatusAPI{
		result: &params.FullStatus{
			Model: params.ModelStatusInfo{
				Name:     "test",
				CloudTag: "cloud-foo",
			},
		},
	}
	s.clock = &timeRecorder{}
	s.SetModelAndController(c, "test", "admin/test")
}

func (s *MinimalStatusSuite) runStatus(c *gc.C, args ...string) (*cmd.Context, error) {
	statusCmd := status.NewTestStatusCommand(s.statusapi, s.clock)
	return cmdtesting.RunCommand(c, statusCmd, args...)
}

func (s *MinimalStatusSuite) TestGoodCall(c *gc.C) {
	_, err := s.runStatus(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.statusapi.calls, gc.Equals, 1)
	c.Check(s.clock.waits, gc.HasLen, 0)
}

func (s *MinimalStatusSuite) TestGoodCallWithStorage(c *gc.C) {
	t := time.Now()
	s.statusapi.expectIncludeStorage = true
	s.statusapi.result.Storage = storageDetails(t)
	s.statusapi.result.Filesystems = filesystemDetails(t)
	s.statusapi.result.Volumes = volumeDetails(t)

	context, err := s.runStatus(c, "--no-color", "--storage")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.statusapi.calls, gc.Equals, 1)
	c.Check(s.clock.waits, gc.HasLen, 0)

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, gc.Equals, `
Model  Controller  Cloud/Region  Version
test   test        foo           

Storage Unit  Storage ID    Type        Pool      Mountpoint  Size     Status    Message
              persistent/1  filesystem                                 detached  
postgresql/0  db-dir/1100   block                             3.0 MiB  attached  
transcode/0   db-dir/1000   block                                      pending   creating volume
transcode/0   shared-fs/0   filesystem  radiance  /mnt/doom   1.0 GiB  attached  
transcode/1   shared-fs/0   filesystem  radiance  /mnt/huang  1.0 GiB  attached  
`[1:])
}

func (s *MinimalStatusSuite) TestRetryOnError(c *gc.C) {
	s.statusapi.errors = []error{
		errors.New("boom"),
		errors.New("splat"),
	}

	_, err := s.runStatus(c, "--no-color")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.statusapi.calls, gc.Equals, 3)
	delay := 100 * time.Millisecond
	c.Check(s.clock.waits, jc.DeepEquals, []time.Duration{delay, delay})
}

func (s *MinimalStatusSuite) TestRetryDelays(c *gc.C) {
	s.statusapi.errors = []error{
		errors.New("boom"),
		errors.New("splat"),
	}

	_, err := s.runStatus(c, "--no-color", "--retry-delay", "250ms")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.statusapi.calls, gc.Equals, 3)
	delay := 250 * time.Millisecond
	c.Check(s.clock.waits, jc.DeepEquals, []time.Duration{delay, delay})
}

func (s *MinimalStatusSuite) TestRetryCount(c *gc.C) {
	s.statusapi.errors = []error{
		errors.New("error 1"),
		errors.New("error 2"),
		errors.New("error 3"),
		errors.New("error 4"),
		errors.New("error 5"),
		errors.New("error 6"),
		errors.New("error 7"),
	}

	_, err := s.runStatus(c, "--no-color", "--retry-count", "5")
	c.Assert(err, gc.ErrorMatches, "error 6")
	c.Check(s.statusapi.calls, gc.Equals, 6)
	delay := 100 * time.Millisecond
	c.Check(s.clock.waits, jc.DeepEquals, []time.Duration{delay, delay, delay, delay, delay})
}

func (s *MinimalStatusSuite) TestRetryCountOfZero(c *gc.C) {
	s.statusapi.errors = []error{
		errors.New("error 1"),
		errors.New("error 2"),
	}

	_, err := s.runStatus(c, "--no-color", "--retry-count", "0")
	c.Assert(err, gc.ErrorMatches, "error 1")
	c.Check(s.statusapi.calls, gc.Equals, 1)
	c.Check(s.clock.waits, gc.HasLen, 0)
}

func (s *MinimalStatusSuite) TestUnauthorizedErrorIsNotRetried(c *gc.C) {
	rpcErr := &params.Error{Code: params.CodeUnauthorized, Message: "permission denied"}
	loginErr := errors.Unauthorizedf("attempted login as %q for user %q", "regular-user", "service-account")
	tests := []struct {
		name          string
		apiErrors     []error
		expectedError error
		expectedCalls int
		expectedWaits []time.Duration
	}{{
		name:          "RPC unauthorized",
		apiErrors:     []error{rpcErr},
		expectedError: rpcErr,
		expectedCalls: 1,
	}, {
		name:          "wrapped RPC unauthorized",
		apiErrors:     []error{errors.Annotate(rpcErr, "getting status")},
		expectedError: rpcErr,
		expectedCalls: 1,
	}, {
		name:          "RPC unauthorized after transient error",
		apiErrors:     []error{errors.New("boom"), rpcErr},
		expectedError: rpcErr,
		expectedCalls: 2,
		expectedWaits: []time.Duration{100 * time.Millisecond},
	}, {
		name:          "login mismatch",
		apiErrors:     []error{loginErr, loginErr, loginErr, loginErr},
		expectedError: loginErr,
		expectedCalls: 1,
	}, {
		name:          "wrapped login mismatch",
		apiErrors:     []error{errors.Trace(errors.Annotate(loginErr, "getting status"))},
		expectedError: loginErr,
		expectedCalls: 1,
	}, {
		name:          "login mismatch after transient error",
		apiErrors:     []error{errors.New("boom"), loginErr, loginErr, loginErr},
		expectedError: loginErr,
		expectedCalls: 2,
		expectedWaits: []time.Duration{100 * time.Millisecond},
	}}
	for _, test := range tests {
		s.statusapi.errors = test.apiErrors
		s.statusapi.calls = 0
		s.clock = &timeRecorder{}
		comment := gc.Commentf("%s", test.name)

		context, err := s.runStatus(c, "--no-color")
		c.Check(err, jc.ErrorIs, test.expectedError, comment)
		c.Check(s.statusapi.calls, gc.Equals, test.expectedCalls, comment)
		c.Check(s.clock.waits, jc.DeepEquals, test.expectedWaits, comment)
		c.Check(cmdtesting.Stdout(context), gc.Equals, "", comment)
	}
}

func (s *MinimalStatusSuite) TestUnauthorizedMessageWithoutCodeIsRetried(c *gc.C) {
	s.statusapi.errors = []error{errors.New(params.CodeUnauthorized)}

	_, err := s.runStatus(c, "--no-color")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.statusapi.calls, gc.Equals, 2)
	c.Check(s.clock.waits, jc.DeepEquals, []time.Duration{100 * time.Millisecond})
}

type fakeStatusAPI struct {
	expectIncludeStorage bool
	result               *params.FullStatus
	errors               []error
	calls                int
}

func (f *fakeStatusAPI) Status(args *client.StatusArgs) (*params.FullStatus, error) {
	f.calls++
	if f.expectIncludeStorage != args.IncludeStorage {
		return nil, errors.New("IncludeStorage arg mismatch")
	}
	if len(f.errors) > 0 {
		err, rest := f.errors[0], f.errors[1:]
		f.errors = rest

		if err != nil {
			return nil, err
		}
	}
	return f.result, nil
}

func (*fakeStatusAPI) Close() error {
	return nil
}

type timeRecorder struct {
	waits  []time.Duration
	result chan time.Time
}

func (r *timeRecorder) After(d time.Duration) <-chan time.Time {
	r.waits = append(r.waits, d)
	if r.result == nil {
		// If we haven't yet, make a closed time channel so it immediately
		// passes.
		r.result = make(chan time.Time)
		close(r.result)
	}
	return r.result
}

func storageDetails(t time.Time) []params.StorageDetails {
	return []params.StorageDetails{{
		StorageTag: "storage-db-dir-1000",
		OwnerTag:   "unit-transcode-0",
		Kind:       params.StorageKindBlock,
		Status: params.EntityStatus{
			Status: corestatus.Pending,
			Since:  &t,
			Info:   "creating volume",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-transcode-0": {
				Location: "thither",
			},
		},
	}, {
		StorageTag: "storage-db-dir-1100",
		OwnerTag:   "unit-postgresql-0",
		Kind:       params.StorageKindBlock,
		Life:       "dying",
		Status: params.EntityStatus{
			Status: corestatus.Attached,
			Since:  &t,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-postgresql-0": {
				Location: "hither",
				Life:     "dying",
			},
		},
	}, {
		StorageTag: "storage-shared-fs-0",
		OwnerTag:   "application-transcode",
		Kind:       params.StorageKindFilesystem,
		Status: params.EntityStatus{
			Status: corestatus.Attached,
			Since:  &t,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-transcode-0": {
				Location: "there",
			},
			"unit-transcode-1": {
				Location: "here",
			},
		},
	}, {
		StorageTag: "storage-persistent-1",
		Kind:       params.StorageKindFilesystem,
		Status: params.EntityStatus{
			Status: corestatus.Detached,
			Since:  &t,
		},
		Persistent: true,
	}}
}

func filesystemDetails(t time.Time) []params.FilesystemDetails {
	return []params.FilesystemDetails{
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
			Status: createTestStatus(corestatus.Attached, "", t),
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
				Status:     createTestStatus(corestatus.Attached, "", t),
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
			Status: createTestStatus(corestatus.Attaching, "failed to attach, will retry", t),
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
			Status: createTestStatus(corestatus.Pending, "", t),
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
			Status: createTestStatus(corestatus.Attached, "", t),
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
			Status: createTestStatus(corestatus.Attached, "", t),
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
				Status:     createTestStatus(corestatus.Attached, "", t),
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
			Status: createTestStatus(corestatus.Attached, "", t),
			Storage: &params.StorageDetails{
				StorageTag: "storage-db-dir-1100",
				OwnerTag:   "unit-abc-0",
				Kind:       params.StorageKindBlock,
				Life:       "alive",
				Status:     createTestStatus(corestatus.Attached, "", t),
				Attachments: map[string]params.StorageAttachmentDetails{
					"unit-abc-0": {
						StorageTag: "storage-db-dir-1100",
						UnitTag:    "unit-abc-0",
						Location:   "/mnt/fuji",
					},
				},
			},
		},
	}
}

func volumeDetails(t time.Time) []params.VolumeDetails {
	return []params.VolumeDetails{
		// volume 0/0 is attached to machine 0, assigned to
		// storage db-dir/1001, which is attached to unit
		// abc/0.
		{
			VolumeTag: "volume-0-0",
			Info: params.VolumeInfo{
				VolumeId: "provider-supplied-volume-0-0",
				Pool:     "radiance",
				Size:     512,
			},
			Life:   "alive",
			Status: createTestStatus(corestatus.Attached, "", t),
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
				Status:     createTestStatus(corestatus.Attached, "", t),
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
				VolumeId:   "provider-supplied-volume-1",
				HardwareId: "serial blah blah",
				Persistent: true,
				Size:       2048,
			},
			Status: createTestStatus(corestatus.Attaching, "failed to attach, will retry", t),
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
			Status: createTestStatus(corestatus.Pending, "", t),
			MachineAttachments: map[string]params.VolumeAttachmentDetails{
				"machine-1": {},
			},
		},
		// volume 2 is due to be attached to machine 1, but is not
		// assigned to any storage and has not yet been provisioned.
		{
			VolumeTag: "volume-2",
			Info: params.VolumeInfo{
				VolumeId: "provider-supplied-volume-2",
				Size:     3,
			},
			Status: createTestStatus(corestatus.Attached, "", t),
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
				VolumeId:   "provider-supplied-volume-4",
				Persistent: true,
				Size:       1024,
			},
			Status: createTestStatus(corestatus.Attached, "", t),
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
				Status:     createTestStatus(corestatus.Attached, "", t),
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
	}
}

func createTestStatus(testStatus corestatus.Status, message string, since time.Time) params.EntityStatus {
	return params.EntityStatus{
		Status: testStatus,
		Info:   message,
		Since:  &since,
	}
}
