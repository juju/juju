// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"errors"
	"time"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/diskformatter"
)

var _ = gc.Suite(&DiskFormatterWorkerSuite{})

type DiskFormatterWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskFormatterWorkerSuite) TestWorker(c *gc.C) {
	volumeAttachments := []params.VolumeAttachment{
		{VolumeTag: "volume-0"},
		{VolumeTag: "volume-1"},
		{VolumeTag: "volume-2"},
		{VolumeTag: "volume-3"},
	}

	volumeFormattingInfoResults := []params.VolumePreparationInfoResult{{
		Result: params.VolumePreparationInfo{
			DevicePath:      "/dev/xvdf1",
			NeedsFilesystem: false,
		},
	}, {
		Result: params.VolumePreparationInfo{
			DevicePath:      "/dev/xvdf2",
			NeedsFilesystem: true,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "volume 2 not found",
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotAssigned,
			Message: "volume 2 not assigned",
		},
	}}

	accessor := &mockVolumeAccessor{
		changes: make(chan struct{}),
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return volumeAttachments, nil
		},
		volumeFormattingInfo: func(tags []names.VolumeTag) ([]params.VolumePreparationInfoResult, error) {
			expect := make([]names.VolumeTag, len(volumeAttachments))
			for i, att := range volumeAttachments {
				tag, err := names.ParseTag(att.VolumeTag)
				c.Assert(err, jc.ErrorIsNil)
				expect[i] = tag.(names.VolumeTag)
			}
			c.Assert(tags, gc.DeepEquals, expect)
			return volumeFormattingInfoResults, nil
		},
	}

	testing.PatchExecutableAsEchoArgs(c, s, "mkfs.ext4")

	w := diskformatter.NewWorker(accessor)
	accessor.changes <- struct{}{}
	done := make(chan struct{})
	go func() {
		w.Kill()
		w.Wait()
		close(done)
	}()

	select {
	case <-done:
		testing.AssertEchoArgs(c, "mkfs.ext4", "/dev/xvdf2")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for diskformatter to update")
	}
}

func (s *DiskFormatterWorkerSuite) TestMakeDefaultFilesystem(c *gc.C) {
	accessor := &mockVolumeAccessor{
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return []params.VolumeAttachment{{
				VolumeTag: "volume-0",
			}}, nil
		},
		volumeFormattingInfo: func(tags []names.VolumeTag) ([]params.VolumePreparationInfoResult, error) {
			return []params.VolumePreparationInfoResult{{
				Result: params.VolumePreparationInfo{
					NeedsFilesystem: true,
					DevicePath:      "/dev/xvdf1",
				},
			}}, nil
		},
	}

	testing.PatchExecutableAsEchoArgs(c, s, "mkfs.ext4")
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle()
	c.Assert(err, gc.IsNil)
	testing.AssertEchoArgs(c, "mkfs.ext4", "/dev/xvdf1")
}

func (s *DiskFormatterWorkerSuite) TestAttachedVolumesError(c *gc.C) {
	accessor := &mockVolumeAccessor{
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return nil, errors.New("AttachedVolumes failed")
		},
	}
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle()
	c.Assert(err, gc.ErrorMatches, "getting attached volumes: AttachedVolumes failed")
}

func (s *DiskFormatterWorkerSuite) TestBlockDeviceStorageInstanceError(c *gc.C) {
	accessor := &mockVolumeAccessor{
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return []params.VolumeAttachment{{VolumeTag: "volume-0"}}, nil
		},
		volumeFormattingInfo: func(tags []names.VolumeTag) ([]params.VolumePreparationInfoResult, error) {
			return []params.VolumePreparationInfoResult{}, errors.New("VolumePreparationInfo failed")
		},
	}
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle()
	c.Assert(err, gc.ErrorMatches, "getting volume formatting info: VolumePreparationInfo failed")
}

func (s *DiskFormatterWorkerSuite) TestCannotMakeFilesystem(c *gc.C) {
	accessor := &mockVolumeAccessor{
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return []params.VolumeAttachment{{VolumeTag: "volume-0"}}, nil
		},
		volumeFormattingInfo: func(tags []names.VolumeTag) ([]params.VolumePreparationInfoResult, error) {
			return []params.VolumePreparationInfoResult{{
				Result: params.VolumePreparationInfo{
					NeedsFilesystem: true,
					DevicePath:      "/dev/xvdf1",
				},
			}}, nil
		},
	}
	// Failure to create a filesystem should not cause the handler to error.
	testing.PatchExecutableThrowError(c, s, "mkfs.ext4", 1)
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle()
	c.Assert(err, jc.ErrorIsNil)
}

type mockVolumeAccessor struct {
	changes              chan struct{}
	attachedVolumes      func() ([]params.VolumeAttachment, error)
	volumeFormattingInfo func([]names.VolumeTag) ([]params.VolumePreparationInfoResult, error)
}

func (m *mockVolumeAccessor) Changes() <-chan struct{} {
	return m.changes
}

func (m *mockVolumeAccessor) Stop() error {
	return nil
}

func (m *mockVolumeAccessor) Err() error {
	return nil
}

func (m *mockVolumeAccessor) WatchAttachedVolumes() (watcher.NotifyWatcher, error) {
	return m, nil
}

func (m *mockVolumeAccessor) AttachedVolumes() ([]params.VolumeAttachment, error) {
	return m.attachedVolumes()
}

func (m *mockVolumeAccessor) VolumePreparationInfo(tags []names.VolumeTag) ([]params.VolumePreparationInfoResult, error) {
	return m.volumeFormattingInfo(tags)
}
