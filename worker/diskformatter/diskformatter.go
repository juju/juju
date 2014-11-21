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

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.diskformatter")

// AttachedBlockDeviceWatcher is an interface used to watch and retrieve details of
// the block devices attached to datastores owned by the authenticated unit agent.
type AttachedBlockDeviceWatcher interface {
	WatchAttachedBlockDevices() (watcher.NotifyWatcher, error)
	AttachedBlockDevices() ([]storage.BlockDevice, error)
}

// BlockDeviceDatastoreGetter is an interface used to retrieve details of the
// datastores that the specified block devices are attached to.
type BlockDeviceDatastoreGetter interface {
	BlockDeviceDatastores([]storage.BlockDeviceId) (params.DatastoreResults, error)
}

// DatastoreFilesystemSetter is an interface used to record information about the
// filesystems created for the specified datastores.
type DatastoreFilesystemSetter interface {
	SetDatastoreFilesystems([]params.DatastoreFilesystem) error
}

// NewWorker returns a new worker that creates filesystems on block devices
// assigned to this unit's datastores.
func NewWorker(
	watcher AttachedBlockDeviceWatcher,
	getter BlockDeviceDatastoreGetter,
	setter DatastoreFilesystemSetter,
) worker.Worker {
	return worker.NewNotifyWorker(newDiskFormatter(watcher, getter, setter))
}

func newDiskFormatter(
	watcher AttachedBlockDeviceWatcher,
	getter BlockDeviceDatastoreGetter,
	setter DatastoreFilesystemSetter,
) worker.NotifyWatchHandler {
	return &diskFormatter{watcher, getter, setter}
}

type diskFormatter struct {
	watcher AttachedBlockDeviceWatcher
	getter  BlockDeviceDatastoreGetter
	setter  DatastoreFilesystemSetter
}

func (f *diskFormatter) SetUp() (watcher.NotifyWatcher, error) {
	return f.watcher.WatchAttachedBlockDevices()
}

func (f *diskFormatter) TearDown() error {
	return nil
}

func (f *diskFormatter) Handle() error {
	// getAttachedBlockDevices returns the block devices that
	// are present in the "machine block devices" subdoc;
	// i.e. those that are attached and visible to the machine.
	blockDevices, err := f.watcher.AttachedBlockDevices()
	if err != nil {
		return errors.Annotate(err, "cannot get block devices")
	}

	blockDeviceIds := make([]storage.BlockDeviceId, len(blockDevices))
	for i, dev := range blockDevices {
		blockDeviceIds[i] = dev.Id
	}
	results, err := f.getter.BlockDeviceDatastores(blockDeviceIds)
	if err != nil {
		return errors.Annotate(err, "cannot get assigned datastores")
	}

	var filesystems []params.DatastoreFilesystem
	for i, result := range results.Results {
		if result.Error != nil {
			// Ignore unassigned block devices; this could happen if
			// the block device were unassigned from a datastore after
			// the initial "AttachedBlockDevices" call returned.
			if !params.IsCodeNotAssigned(result.Error) {
				logger.Errorf(
					"could not determine datastore for block device %q: %v",
					blockDeviceIds[i], result.Error,
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
			logger.Debugf("block device %q already has a filesystem", blockDeviceIds[i])
			continue
		}
		devicePath, err := storage.BlockDevicePath(blockDevices[i])
		if err != nil {
			logger.Errorf("cannot get path for block device %q: %v", blockDeviceIds[i], err)
			continue
		}
		pref := createFilesystem(devicePath, datastore.Specification)
		if pref == nil {
			logger.Errorf("failed to create filesystem on block device %q", blockDeviceIds[i])
			continue
		}
		filesystems = append(filesystems, params.DatastoreFilesystem{
			DatastoreId: datastore.Id,
			Filesystem:  pref.Filesystem,
		})
	}

	if len(filesystems) > 0 {
		if err := f.setter.SetDatastoreFilesystems(filesystems); err != nil {
			return errors.Annotate(err, "cannot set datastore filesystems")
		}
	}
	return nil
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
