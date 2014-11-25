// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/diskformatter"
	"github.com/juju/testing"
)

var _ = gc.Suite(&DiskFormatterWorkerSuite{})

type DiskFormatterWorkerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskFormatterWorkerSuite) TestWorker(c *gc.C) {
	blockDeviceIds := []storage.BlockDeviceId{
		"dev0", "dev1", "dev2", "dev3", "dev4", "dev5",
	}
	attachedBlockDeviceIds := blockDeviceIds[:5]

	attachedBlockDeviceResults := []params.BlockDeviceResult{{
		Result: storage.BlockDevice{
			Id:         "dev0",
			DeviceName: "sda",
			Label:      "dev0-label",
			UUID:       "9aade75c-6528-4acf-ab69-258b8dc51798",
		},
	}, {
		Result: storage.BlockDevice{
			Id:         "dev1",
			DeviceName: "sdb",
			UUID:       "edc2348a-a2bc-4b15-b7f3-cb951b3489ad",
		},
	}, {
		Result: storage.BlockDevice{
			Id:         "dev2",
			DeviceName: "sdc",
		},
	}, {
		Result: storage.BlockDevice{
			Id:         "dev3",
			DeviceName: "sdd",
		},
	}, {
		Result: storage.BlockDevice{
			Id:         "dev4",
			DeviceName: "sde",
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "dev5 not found",
		},
	}}

	blockDeviceDatastoreResults := []params.DatastoreResult{{
		Result: storage.Datastore{
			Id:   "needs-a-filesystem",
			Name: "shared-fs",
			Kind: storage.DatastoreKindFilesystem,
			Specification: &storage.Specification{
				FilesystemPreferences: []storage.FilesystemPreference{{
					Filesystem: storage.Filesystem{
						Type: "afs",
					},
				}, {
					Filesystem: storage.Filesystem{
						Type:         "btrfs",
						MountOptions: []string{"autodefrag"},
					},
					MkfsOptions: []string{"--mixed"},
				}},
			},
		},
	}, {
		Result: storage.Datastore{
			Id:         "already-has-a-filesystem",
			Name:       "shared-fs",
			Kind:       storage.DatastoreKindFilesystem,
			Filesystem: &storage.Filesystem{Type: "btrfs"},
		},
	}, {
		Result: storage.Datastore{
			Id:   "doesnt-need-a-filesystem",
			Name: "blocks",
			Kind: storage.DatastoreKindBlock,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotAssigned,
			Message: "dev3 is not assigned to a datastore",
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "dev4 not found",
		},
	}}

	watcher := &mockAttachedBlockDeviceWatcher{
		changes: make(chan []string),
		stopped: make(chan struct{}),
		attachedBlockDevices: func(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
			c.Assert(ids, gc.DeepEquals, blockDeviceIds)
			return params.BlockDeviceResults{attachedBlockDeviceResults}, nil
		},
	}

	var getter blockDeviceDatastoreGetterFunc = func(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
		c.Assert(ids, gc.DeepEquals, attachedBlockDeviceIds)
		return params.DatastoreResults{blockDeviceDatastoreResults}, nil
	}

	testing.PatchExecutableThrowError(c, s, "mkfs.afs", 1)
	testing.PatchExecutableAsEchoArgs(c, s, "mkfs.btrfs")

	done := make(chan struct{})
	var setter datastoreFilesystemSetterFunc = func(fs []params.DatastoreFilesystem) error {
		c.Assert(fs, gc.DeepEquals, []params.DatastoreFilesystem{{
			DatastoreId: "needs-a-filesystem",
			Filesystem: storage.Filesystem{
				Type:         "btrfs",
				MountOptions: []string{"autodefrag"},
			},
		}})
		testing.AssertEchoArgs(c, "mkfs.btrfs", "--mixed", "/dev/disk/by-label/dev0-label")
		close(done)
		return nil
	}

	w := diskformatter.NewWorker(watcher, getter, setter)
	defer w.Wait()
	defer w.Kill()

	changes := make([]string, len(blockDeviceIds))
	for i, id := range blockDeviceIds {
		changes[i] = string(id)
	}
	watcher.changes <- changes

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for diskformatter to update")
	}
}

func (s *DiskFormatterWorkerSuite) TestMakeDefaultFilesystem(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{
					Id:         "dev0",
					DeviceName: "sda",
					Label:      "dev0-label",
					UUID:       "9aade75c-6528-4acf-ab69-258b8dc51798",
				},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
		return params.DatastoreResults{[]params.DatastoreResult{{
			Result: storage.Datastore{
				Id:   "needs-a-filesystem",
				Name: "shared-fs",
				Kind: storage.DatastoreKindFilesystem,
				Specification: &storage.Specification{
					FilesystemPreferences: []storage.FilesystemPreference{{
						Filesystem: storage.Filesystem{Type: "afs"},
					}},
				},
			},
		}}}, nil
	}
	var called bool
	var setter datastoreFilesystemSetterFunc = func(fs []params.DatastoreFilesystem) error {
		c.Assert(fs, gc.DeepEquals, []params.DatastoreFilesystem{{
			DatastoreId: "needs-a-filesystem",
			Filesystem:  storage.Filesystem{Type: storage.DefaultFilesystemType},
		}})
		called = true
		return nil
	}

	testing.PatchExecutableThrowError(c, s, "mkfs.afs", 1)
	testing.PatchExecutableAsEchoArgs(c, s, "mkfs."+storage.DefaultFilesystemType)
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"dev0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *DiskFormatterWorkerSuite) TestAttachedBlockDevicesError(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{}, errors.New("AttachedBlockDevices failed")
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
		c.Fatalf("BlockDeviceDatastores should not be called")
		return params.DatastoreResults{}, nil
	}
	var setter datastoreFilesystemSetterFunc = func(fs []params.DatastoreFilesystem) error {
		c.Fatalf("SetDatastoreFilesystems should not be called")
		return nil
	}
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"dev0"})
	c.Assert(err, gc.ErrorMatches, "cannot get block devices: AttachedBlockDevices failed")
}

func (s *DiskFormatterWorkerSuite) TestBlockDeviceDatastoresError(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Id: "dev0", DeviceName: "sda"},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
		return params.DatastoreResults{}, errors.New("BlockDeviceDatastores failed")
	}
	var setter datastoreFilesystemSetterFunc = func(fs []params.DatastoreFilesystem) error {
		c.Fatalf("SetDatastoreFilesystems should not be called")
		return nil
	}
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"dev0"})
	c.Assert(err, gc.ErrorMatches, "cannot get assigned datastores: BlockDeviceDatastores failed")
}

func (s *DiskFormatterWorkerSuite) TestSetDatastoreFilesystemsError(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Id: "dev0", DeviceName: "sda"},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
		return params.DatastoreResults{[]params.DatastoreResult{{
			Result: storage.Datastore{
				Id:            "needs-a-filesystem",
				Name:          "shared-fs",
				Kind:          storage.DatastoreKindFilesystem,
				Specification: &storage.Specification{},
			},
		}}}, nil
	}
	var setter datastoreFilesystemSetterFunc = func(fs []params.DatastoreFilesystem) error {
		return errors.New("SetDatastoreFilesystems failed")
	}
	testing.PatchExecutableAsEchoArgs(c, s, "mkfs."+storage.DefaultFilesystemType)
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"dev0"})
	c.Assert(err, gc.ErrorMatches, "cannot set datastore filesystems: SetDatastoreFilesystems failed")
}

func (s *DiskFormatterWorkerSuite) TestCannotMakeFilesystem(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Id: "dev0", DeviceName: "sda"},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
		return params.DatastoreResults{[]params.DatastoreResult{{
			Result: storage.Datastore{
				Id:            "needs-a-filesystem",
				Name:          "shared-fs",
				Kind:          storage.DatastoreKindFilesystem,
				Specification: &storage.Specification{},
			},
		}}}, nil
	}
	var setter datastoreFilesystemSetterFunc = func(fs []params.DatastoreFilesystem) error {
		c.Fatalf("SetDatastoreFilesystems should not be called (mkfs failed)")
		return nil
	}
	// Failure to create a filesystem should not cause the handler to error;
	// we should not see a SetDatastoreFilesystems call for that block device's
	// datastore though.
	testing.PatchExecutableThrowError(c, s, "mkfs."+storage.DefaultFilesystemType, 1)
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"dev0"})
	c.Assert(err, jc.ErrorIsNil)
}

type mockAttachedBlockDeviceWatcher struct {
	changes              chan []string
	stopped              chan struct{}
	attachedBlockDevices func([]storage.BlockDeviceId) (params.BlockDeviceResults, error)
}

func (m *mockAttachedBlockDeviceWatcher) Changes() <-chan []string {
	return m.changes
}

func (m *mockAttachedBlockDeviceWatcher) Stop() error {
	select {
	case <-m.stopped:
	default:
		close(m.stopped)
	}
	return nil
}

func (m *mockAttachedBlockDeviceWatcher) Err() error {
	return nil
}

func (m *mockAttachedBlockDeviceWatcher) WatchAttachedBlockDevices() (watcher.StringsWatcher, error) {
	return m, nil
}

func (m *mockAttachedBlockDeviceWatcher) AttachedBlockDevices(ids []storage.BlockDeviceId) (params.BlockDeviceResults, error) {
	return m.attachedBlockDevices(ids)
}

type blockDeviceDatastoreGetterFunc func([]storage.BlockDeviceId) (params.DatastoreResults, error)

func (f blockDeviceDatastoreGetterFunc) BlockDeviceDatastores(ids []storage.BlockDeviceId) (params.DatastoreResults, error) {
	return f(ids)
}

type datastoreFilesystemSetterFunc func([]params.DatastoreFilesystem) error

func (f datastoreFilesystemSetterFunc) SetDatastoreFilesystems(fs []params.DatastoreFilesystem) error {
	return f(fs)
}
