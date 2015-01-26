// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"errors"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/diskformatter"
	"github.com/juju/testing"
)

var _ = gc.Suite(&DiskFormatterWorkerSuite{})

type DiskFormatterWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskFormatterWorkerSuite) TestWorker(c *gc.C) {
	volumeAttachments := []params.VolumeAttachment{
		{VolumeTag: "disk-0"},
		{VolumeTag: "disk-1"},
		{VolumeTag: "disk-2"},
	}

	volumeFormattingInfoResults := []params.VolumeFormattingInfoResult{{
		Result: params.VolumeFormattingInfo{
			DevicePath:      "/dev/xvdf1",
			NeedsFormatting: false,
		},
	}, {
		Result: params.VolumeFormattingInfo{
			DevicePath:      "/dev/xvdf2",
			NeedsFormatting: true,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "volume 2 not found",
		},
	}}

	accessor := &mockVolumeAccessor{
		changes: make(chan struct{}),
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return volumeAttachments, nil
		},
		volumeFormattingInfo: func(tags []names.DiskTag) ([]params.VolumeFormattingInfoResult, error) {
			expect := make([]names.DiskTag, len(volumeAttachments))
			for i, att := range volumeAttachments {
				tag, err := names.ParseTag(att.VolumeTag)
				c.Assert(err, jc.ErrorIsNil)
				expect[i] = tag.(names.DiskTag)
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
				VolumeTag: "disk-0",
			}}, nil
		},
		volumeFormattingInfo: func(tags []names.DiskTag) ([]params.VolumeFormattingInfoResult, error) {
			return []params.VolumeFormattingInfoResult{{
				Result: params.VolumeFormattingInfo{
					NeedsFormatting: true,
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
			return []params.VolumeAttachment{{VolumeTag: "disk-0"}}, nil
		},
		volumeFormattingInfo: func(tags []names.DiskTag) ([]params.VolumeFormattingInfoResult, error) {
			return []params.VolumeFormattingInfoResult{}, errors.New("VolumeFormattingInfo failed")
		},
	}
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle()
	c.Assert(err, gc.ErrorMatches, "getting volume formatting info: VolumeFormattingInfo failed")
}

func (s *DiskFormatterWorkerSuite) TestCannotMakeFilesystem(c *gc.C) {
	accessor := &mockVolumeAccessor{
		attachedVolumes: func() ([]params.VolumeAttachment, error) {
			return []params.VolumeAttachment{{VolumeTag: "disk-0"}}, nil
		},
		volumeFormattingInfo: func(tags []names.DiskTag) ([]params.VolumeFormattingInfoResult, error) {
			return []params.VolumeFormattingInfoResult{{
				Result: params.VolumeFormattingInfo{
					NeedsFormatting: true,
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
	volumeFormattingInfo func([]names.DiskTag) ([]params.VolumeFormattingInfoResult, error)
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

func (m *mockVolumeAccessor) VolumeFormattingInfo(tags []names.DiskTag) ([]params.VolumeFormattingInfoResult, error) {
	return m.volumeFormattingInfo(tags)
}
