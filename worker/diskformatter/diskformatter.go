// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package diskformatter defines a worker that watches for volume attachments
// owned by the machine that runs this worker, and creates filesystems on
// them as necessary. Each machine agent runs this worker.
package diskformatter

import (
	"bytes"
	"os/exec"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.diskformatter")

// defaultFilesystemType is the default filesystem type to
// create on a managed block device for a "filesystem" type
// storage instance.
const defaultFilesystemType = "ext4"

// VolumeAccessor is an interface used to watch and retrieve details of
// the volumes attached to the machine, and related storage instances.
type VolumeAccessor interface {
	// WatchAttachedVolumes watches for volumes attached to the machine.
	WatchAttachedVolumes() (watcher.NotifyWatcher, error)
	// AttachedVolumes returns the volumes which are attached to the
	// machine.
	AttachedVolumes() ([]params.VolumeAttachment, error)
	// VolumePreparationInfo returns information required to format the
	// specified volumes.
	VolumePreparationInfo([]names.VolumeTag) ([]params.VolumePreparationInfoResult, error)
}

// NewWorker returns a new worker that creates filesystems on volumes
// attached to the machine which are assigned to filesystem-kind storage
// instances.
func NewWorker(accessor VolumeAccessor) worker.Worker {
	return worker.NewNotifyWorker(newDiskFormatter(accessor))
}

func newDiskFormatter(accessor VolumeAccessor) worker.NotifyWatchHandler {
	return &diskFormatter{accessor}
}

type diskFormatter struct {
	accessor VolumeAccessor
}

func (f *diskFormatter) SetUp() (watcher.NotifyWatcher, error) {
	return f.accessor.WatchAttachedVolumes()
}

func (f *diskFormatter) TearDown() error {
	return nil
}

func (f *diskFormatter) Handle() error {
	attachments, err := f.accessor.AttachedVolumes()
	if err != nil {
		return errors.Annotate(err, "getting attached volumes")
	}

	tags := make([]names.VolumeTag, len(attachments))
	for i, info := range attachments {
		tag, err := names.ParseVolumeTag(info.VolumeTag)
		if err != nil {
			return errors.Annotate(err, "parsing disk tag")
		}
		tags[i] = tag
	}
	if len(tags) == 0 {
		return nil
	}

	info, err := f.accessor.VolumePreparationInfo(tags)
	if err != nil {
		return errors.Annotate(err, "getting volume formatting info")
	}

	for i, tag := range tags {
		if info[i].Error != nil {
			if !params.IsCodeNotAssigned(info[i].Error) {
				logger.Errorf(
					"failed to get formatting info for volume %q: %v",
					tag.Id(), info[i].Error,
				)
			}
			continue
		}
		if !info[i].Result.NeedsFilesystem {
			continue
		}
		devicePath := info[i].Result.DevicePath
		if err := createFilesystem(devicePath); err != nil {
			logger.Errorf("failed to create filesystem on volume %q: %v", tag.Id(), err)
			continue
		}
		// Filesystem will be reported by diskmanager.
	}

	return nil
}

func createFilesystem(devicePath string) error {
	logger.Debugf("attempting to create filesystem on %q", devicePath)
	mkfscmd := "mkfs." + defaultFilesystemType
	output, err := exec.Command(mkfscmd, devicePath).CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "%s failed (%q)", mkfscmd, bytes.TrimSpace(output))
	}
	logger.Infof("created filesystem on %q", devicePath)
	return nil
}
