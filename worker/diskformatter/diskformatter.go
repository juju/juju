// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package diskformatter defines a worker that watches for block devices
// assigned to storage instances owned by the unit that runs this worker,
// and creates filesystems on them as necessary. Each unit agent runs this
// worker.
package diskformatter

import (
	"bytes"
	"os/exec"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.diskformatter")

// defaultFilesystemType is the default filesystem type to
// create on a managed block device for a "filesystem" type
// storage instance.
const defaultFilesystemType = "ext4"

// BlockDeviceAccessor is an interface used to watch and retrieve details of
// the block devices assigned to storage instances owned by the unit.
type BlockDeviceAccessor interface {
	WatchBlockDevices() (watcher.StringsWatcher, error)
	BlockDevice([]names.DiskTag) (params.BlockDeviceResults, error)
	BlockDeviceAttached([]names.DiskTag) (params.BoolResults, error)
	BlockDeviceStorageInstance([]names.DiskTag) (params.StorageInstanceResults, error)
}

// NewWorker returns a new worker that creates filesystems on block devices
// assigned to this unit's storage instances.
func NewWorker(
	accessor BlockDeviceAccessor,
) worker.Worker {
	return worker.NewStringsWorker(newDiskFormatter(accessor))
}

func newDiskFormatter(accessor BlockDeviceAccessor) worker.StringsWatchHandler {
	return &diskFormatter{accessor}
}

type diskFormatter struct {
	accessor BlockDeviceAccessor
}

func (f *diskFormatter) SetUp() (watcher.StringsWatcher, error) {
	return f.accessor.WatchBlockDevices()
}

func (f *diskFormatter) TearDown() error {
	return nil
}

func (f *diskFormatter) Handle(diskNames []string) error {
	tags := make([]names.DiskTag, len(diskNames))
	for i, name := range diskNames {
		tags[i] = names.NewDiskTag(name)
	}

	// attachedBlockDevices returns the block devices that are
	// assigned to the caller, and are known to be attached and
	// visible to their associated machines.
	blockDevices, err := f.attachedBlockDevices(tags)
	if err != nil {
		return err
	}

	blockDeviceTags := make([]names.DiskTag, len(blockDevices))
	for i, dev := range blockDevices {
		blockDeviceTags[i] = names.NewDiskTag(dev.Name)
	}

	// Map block devices to the storage instances they are assigned to.
	results, err := f.accessor.BlockDeviceStorageInstance(blockDeviceTags)
	if err != nil {
		return errors.Annotate(err, "cannot get assigned storage instances")
	}

	for i, result := range results.Results {
		if result.Error != nil {
			logger.Errorf(
				"could not determine storage instance for block device %q: %v",
				blockDevices[i].Name, result.Error,
			)
			continue
		}
		if blockDevices[i].FilesystemType != "" {
			logger.Debugf("block device %q already has a filesystem", blockDevices[i].Name)
			continue
		}
		storageInstance := result.Result
		if storageInstance.Kind != storage.StorageKindFilesystem {
			logger.Debugf("storage instance %q does not need a filesystem", storageInstance.Id)
			continue
		}
		devicePath, err := storage.BlockDevicePath(blockDevices[i])
		if err != nil {
			logger.Errorf("cannot get path for block device %q: %v", blockDevices[i].Name, err)
			continue
		}
		if err := createFilesystem(devicePath); err != nil {
			logger.Errorf("failed to create filesystem on block device %q: %v", blockDevices[i].Name, err)
			continue
		}
	}

	return nil
}

func (f *diskFormatter) attachedBlockDevices(tags []names.DiskTag) ([]storage.BlockDevice, error) {
	results, err := f.accessor.BlockDevice(tags)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get block devices")
	}
	attached, err := f.accessor.BlockDeviceAttached(tags)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get block device attachment status")
	}
	if len(results.Results) != len(attached.Results) {
		return nil, errors.New("BlockDevice and BlockDeviceAttached returned a different number of results")
	}
	blockDevices := make([]storage.BlockDevice, 0, len(tags))
	for i := range results.Results {
		result := results.Results[i]
		attached := attached.Results[i]
		if result.Error != nil {
			if !errors.IsNotFound(result.Error) {
				logger.Errorf("could not get details for block device %q", tags[i])
			}
			continue
		}
		if attached.Error != nil {
			if !errors.IsNotFound(attached.Error) {
				logger.Errorf("could not get attachment status for block device %q", tags[i])
			}
			continue
		}
		if attached.Result {
			blockDevices = append(blockDevices, result.Result)
		}
	}
	return blockDevices, nil
}

func createFilesystem(devicePath string) error {
	logger.Debugf("attempting to create filesystem on %q", devicePath)
	if err := maybeCreateFilesystem(devicePath); err != nil {
		return err
	}
	logger.Infof("created filesystem on %q", devicePath)
	return nil
}

func maybeCreateFilesystem(path string) error {
	mkfscmd := "mkfs." + defaultFilesystemType
	output, err := exec.Command(mkfscmd, path).CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "%s failed (%q)", mkfscmd, bytes.TrimSpace(output))
	}
	return nil
}
