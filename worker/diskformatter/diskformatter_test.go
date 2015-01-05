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
	ids := []string{
		"0", "1", "2", "3", "4", "5",
	}

	blockDeviceResults := []params.BlockDeviceResult{{
		Result: storage.BlockDevice{
			Name:       "0",
			DeviceName: "sda",
			Label:      "dev0-label",
			UUID:       "9aade75c-6528-4acf-ab69-258b8dc51798",
		},
	}, {
		Result: storage.BlockDevice{
			Name:       "1",
			DeviceName: "sdb",
			UUID:       "edc2348a-a2bc-4b15-b7f3-cb951b3489ad",
		},
	}, {
		Result: storage.BlockDevice{
			Name:       "2",
			DeviceName: "sdc",
		},
	}, {
		Result: storage.BlockDevice{
			Name:       "3",
			DeviceName: "sdd",
		},
	}, {
		Result: storage.BlockDevice{
			Name:       "4",
			DeviceName: "sde",
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "block device 5 not found",
		},
	}}

	blockDeviceDatastoreResults := []params.DatastoreResult{{
		Result: storage.Datastore{
			Name: "needs-a-filesystem",
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
			Name:       "already-has-a-filesystem",
			Kind:       storage.DatastoreKindFilesystem,
			Filesystem: &storage.Filesystem{Type: "btrfs"},
		},
	}, {
		Result: storage.Datastore{
			Name: "doesnt-need-a-filesystem",
			Kind: storage.DatastoreKindBlock,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotAssigned,
			Message: "3 is not assigned to a datastore",
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "4 not found",
		},
	}}

	watcher := &mockAttachedBlockDeviceWatcher{
		changes: make(chan []string),
		stopped: make(chan struct{}),
		attachedBlockDevices: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			expect := make([]names.DiskTag, len(ids))
			for i, id := range ids {
				expect[i] = names.NewDiskTag(id)
			}
			c.Assert(tags, gc.DeepEquals, expect)
			return params.BlockDeviceResults{blockDeviceResults}, nil
		},
	}

	var getter blockDeviceDatastoreGetterFunc = func(tags []names.DiskTag) (params.DatastoreResults, error) {
		expect := make([]names.DiskTag, 0, len(blockDeviceResults))
		for _, result := range blockDeviceResults {
			if result.Result.Name == "" {
				continue
			}
			expect = append(expect, names.NewDiskTag(result.Result.Name))
		}
		c.Assert(tags, gc.DeepEquals, expect)
		return params.DatastoreResults{blockDeviceDatastoreResults}, nil
	}

	testing.PatchExecutableThrowError(c, s, "mkfs.afs", 1)
	testing.PatchExecutableAsEchoArgs(c, s, "mkfs.btrfs")

	done := make(chan struct{})
	var setter blockDeviceFilesystemSetterFunc = func(fs []params.BlockDeviceFilesystem) error {
		c.Assert(fs, gc.DeepEquals, []params.BlockDeviceFilesystem{{
			DiskTag:   "disk-0",
			Datastore: "needs-a-filesystem",
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
	watcher.changes <- ids

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for diskformatter to update")
	}
}

func (s *DiskFormatterWorkerSuite) TestMakeDefaultFilesystem(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func([]names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{
					Name:       "0",
					DeviceName: "sda",
					Label:      "dev0-label",
					UUID:       "9aade75c-6528-4acf-ab69-258b8dc51798",
				},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(tags []names.DiskTag) (params.DatastoreResults, error) {
		return params.DatastoreResults{[]params.DatastoreResult{{
			Result: storage.Datastore{
				Name: "needs-a-filesystem",
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
	var setter blockDeviceFilesystemSetterFunc = func(fs []params.BlockDeviceFilesystem) error {
		c.Assert(fs, gc.DeepEquals, []params.BlockDeviceFilesystem{{
			DiskTag:    "disk-0",
			Datastore:  "needs-a-filesystem",
			Filesystem: storage.Filesystem{Type: storage.DefaultFilesystemType},
		}})
		called = true
		return nil
	}

	testing.PatchExecutableThrowError(c, s, "mkfs.afs", 1)
	testing.PatchExecutableAsEchoArgs(c, s, "mkfs."+storage.DefaultFilesystemType)
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *DiskFormatterWorkerSuite) TestAttachedBlockDevicesError(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{}, errors.New("AttachedBlockDevices failed")
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(tags []names.DiskTag) (params.DatastoreResults, error) {
		c.Fatalf("BlockDeviceDatastores should not be called")
		return params.DatastoreResults{}, nil
	}
	var setter blockDeviceFilesystemSetterFunc = func(fs []params.BlockDeviceFilesystem) error {
		c.Fatalf("SetBlockDeviceFilesystems should not be called")
		return nil
	}
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.ErrorMatches, "cannot get block devices: AttachedBlockDevices failed")
}

func (s *DiskFormatterWorkerSuite) TestBlockDeviceDatastoresError(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func([]names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Name: "0", DeviceName: "sda"},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(tags []names.DiskTag) (params.DatastoreResults, error) {
		return params.DatastoreResults{}, errors.New("BlockDeviceDatastores failed")
	}
	var setter blockDeviceFilesystemSetterFunc = func(fs []params.BlockDeviceFilesystem) error {
		c.Fatalf("SetBlockDeviceFilesystems should not be called")
		return nil
	}
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.ErrorMatches, "cannot get assigned datastores: BlockDeviceDatastores failed")
}

func (s *DiskFormatterWorkerSuite) TestSetBlockDeviceFilesystemsError(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Name: "0", DeviceName: "sda"},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(tags []names.DiskTag) (params.DatastoreResults, error) {
		return params.DatastoreResults{[]params.DatastoreResult{{
			Result: storage.Datastore{
				Name:          "needs-a-filesystem",
				Kind:          storage.DatastoreKindFilesystem,
				Specification: &storage.Specification{},
			},
		}}}, nil
	}
	var setter blockDeviceFilesystemSetterFunc = func(fs []params.BlockDeviceFilesystem) error {
		return errors.New("SetBlockDeviceFilesystems failed")
	}
	testing.PatchExecutableAsEchoArgs(c, s, "mkfs."+storage.DefaultFilesystemType)
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.ErrorMatches, "cannot set filesystems: SetBlockDeviceFilesystems failed")
}

func (s *DiskFormatterWorkerSuite) TestCannotMakeFilesystem(c *gc.C) {
	watcher := &mockAttachedBlockDeviceWatcher{
		attachedBlockDevices: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Name: "0", DeviceName: "sda"},
			}}}, nil
		},
	}
	var getter blockDeviceDatastoreGetterFunc = func(tags []names.DiskTag) (params.DatastoreResults, error) {
		return params.DatastoreResults{[]params.DatastoreResult{{
			Result: storage.Datastore{
				Name:          "needs-a-filesystem",
				Kind:          storage.DatastoreKindFilesystem,
				Specification: &storage.Specification{},
			},
		}}}, nil
	}
	var setter blockDeviceFilesystemSetterFunc = func(fs []params.BlockDeviceFilesystem) error {
		c.Fatalf("SetBlockDeviceFilesystems should not be called (mkfs failed)")
		return nil
	}
	// Failure to create a filesystem should not cause the handler to error;
	// we should not see a SetBlockDeviceFilesystems call for that block device's
	// datastore though.
	testing.PatchExecutableThrowError(c, s, "mkfs."+storage.DefaultFilesystemType, 1)
	formatter := diskformatter.NewDiskFormatter(watcher, getter, setter)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.IsNil)
}

type mockAttachedBlockDeviceWatcher struct {
	changes              chan []string
	stopped              chan struct{}
	attachedBlockDevices func([]names.DiskTag) (params.BlockDeviceResults, error)
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

func (m *mockAttachedBlockDeviceWatcher) BlockDevice(tags []names.DiskTag) (params.BlockDeviceResults, error) {
	return m.attachedBlockDevices(tags)
}

type blockDeviceDatastoreGetterFunc func([]names.DiskTag) (params.DatastoreResults, error)

func (f blockDeviceDatastoreGetterFunc) BlockDeviceDatastore(tags []names.DiskTag) (params.DatastoreResults, error) {
	return f(tags)
}

type blockDeviceFilesystemSetterFunc func([]params.BlockDeviceFilesystem) error

func (f blockDeviceFilesystemSetterFunc) SetBlockDeviceFilesystem(fs []params.BlockDeviceFilesystem) error {
	return f(fs)
}
