// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package diskformatter defines a worker that watches for block devices
// attached to datastores owned by the unit that runs this worker, and
// creates filesystems on them as necessary. Each unit agent runs this
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

// AttachedBlockDeviceWatcher is an interface used to watch and retrieve details of
// the block devices attached to datastores owned by the authenticated unit agent.
type AttachedBlockDeviceWatcher interface {
	WatchAttachedBlockDevices() (watcher.StringsWatcher, error)
	BlockDevice([]names.DiskTag) (params.BlockDeviceResults, error)
}

// BlockDeviceDatastoreGetter is an interface used to retrieve details of the
// datastores that the specified block devices are attached to.
type BlockDeviceDatastoreGetter interface {
	BlockDeviceDatastore([]names.DiskTag) (params.DatastoreResults, error)
}

// BlockDeviceFilesystemSetter is an interface used to record information
// about the filesystems created for the specified block devices.
type BlockDeviceFilesystemSetter interface {
	SetBlockDeviceFilesystem([]params.BlockDeviceFilesystem) error
}

// NewWorker returns a new worker that creates filesystems on block devices
// assigned to this unit's datastores.
func NewWorker(
	watcher AttachedBlockDeviceWatcher,
	getter BlockDeviceDatastoreGetter,
	setter BlockDeviceFilesystemSetter,
) worker.Worker {
	return worker.NewStringsWorker(newDiskFormatter(watcher, getter, setter))
}

func newDiskFormatter(
	watcher AttachedBlockDeviceWatcher,
	getter BlockDeviceDatastoreGetter,
	setter BlockDeviceFilesystemSetter,
) worker.StringsWatchHandler {
	return &diskFormatter{watcher, getter, setter}
}

type diskFormatter struct {
	watcher AttachedBlockDeviceWatcher
	getter  BlockDeviceDatastoreGetter
	setter  BlockDeviceFilesystemSetter
}

func (f *diskFormatter) SetUp() (watcher.StringsWatcher, error) {
	return f.watcher.WatchAttachedBlockDevices()
}

func (f *diskFormatter) TearDown() error {
	return nil
}

func (f *diskFormatter) Handle(diskNames []string) error {
	tags := make([]names.DiskTag, len(diskNames))
	for i, name := range diskNames {
		tags[i] = names.NewDiskTag(name)
	}

	// attachedBlockDevices returns the block devices that
	// are present in the "machine block devices" subdoc;
	// i.e. those that are attached and visible to the machine.
	blockDevices, err := f.attachedBlockDevices(tags)
	if err != nil {
		return err
	}

	blockDeviceTags := make([]names.DiskTag, len(blockDevices))
	for i, dev := range blockDevices {
		blockDeviceTags[i] = names.NewDiskTag(dev.Name)
	}

	// Map block devices to the datastores they are assigned to.
	results, err := f.getter.BlockDeviceDatastore(blockDeviceTags)
	if err != nil {
		return errors.Annotate(err, "cannot get assigned datastores")
	}

	var filesystems []params.BlockDeviceFilesystem
	for i, result := range results.Results {
		if result.Error != nil {
			// Ignore unassigned block devices; this could happen if
			// the block device were unassigned from a datastore after
			// the initial "BlockDevice" call returned.
			if !params.IsCodeNotAssigned(result.Error) {
				logger.Errorf(
					"could not determine datastore for block device %q: %v",
					blockDevices[i].Name, result.Error,
				)
			}
			continue
		}
		datastore := result.Result
		if datastore.Kind != storage.DatastoreKindFilesystem {
			logger.Debugf("datastore %q does not need a filesystem", datastore.Name)
			continue
		}
		if datastore.Filesystem != nil {
			logger.Debugf("block device %q already has a filesystem", blockDevices[i].Name)
			continue
		}
		devicePath, err := storage.BlockDevicePath(blockDevices[i])
		if err != nil {
			logger.Errorf("cannot get path for block device %q: %v", blockDevices[i].Name, err)
			continue
		}
		pref := createFilesystem(devicePath, datastore.Specification)
		if pref == nil {
			logger.Errorf("failed to create filesystem on block device %q", blockDevices[i].Name)
			continue
		}
		filesystems = append(filesystems, params.BlockDeviceFilesystem{
			// We must specify both blockdevice and datastore, in case the
			// blockdevice is unassigned or reassigned to another datastore.
			DiskTag:    blockDeviceTags[i].String(),
			Datastore:  datastore.Name,
			Filesystem: pref.Filesystem,
		})
	}

	if len(filesystems) > 0 {
		if err := f.setter.SetBlockDeviceFilesystem(filesystems); err != nil {
			return errors.Annotate(err, "cannot set filesystems")
		}
	}
	return nil
}

func (f *diskFormatter) attachedBlockDevices(tags []names.DiskTag) ([]storage.BlockDevice, error) {
	results, err := f.watcher.BlockDevice(tags)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get block devices")
	}
	blockDevices := make([]storage.BlockDevice, 0, len(tags))
	for i, result := range results.Results {
		if result.Error != nil {
			if !errors.IsNotFound(result.Error) {
				logger.Errorf("could not get details for block device %q", tags[i])
			}
			continue
		}
		blockDevices = append(blockDevices, result.Result)
	}
	return blockDevices, nil
}

func createFilesystem(devicePath string, spec *storage.Specification) *storage.FilesystemPreference {
	prefs := spec.FilesystemPreferences
	prefs = append(prefs, storage.FilesystemPreference{
		Filesystem: storage.Filesystem{
			Type: storage.DefaultFilesystemType,
		},
	})
	for _, pref := range prefs {
		logger.Debugf("attempting to create %q filesystem on %q", pref.Type, devicePath)
		if err := maybeCreateFilesystem(devicePath, pref); err == nil {
			logger.Infof("created %q filesystem on %q", pref.Type, devicePath)
			return &pref
		}
	}
	return nil
}

func maybeCreateFilesystem(path string, fs storage.FilesystemPreference) error {
	mkfscmd := "mkfs." + fs.Type
	args := append([]string{}, fs.MkfsOptions...)
	args = append(args, path)
	output, err := exec.Command(mkfscmd, args...).CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "%s failed (%q)", mkfscmd, bytes.TrimSpace(output))
	}
	return nil
}
