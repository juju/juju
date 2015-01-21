// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"errors"
	"time"

	"github.com/juju/names"
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
			Name:           "1",
			DeviceName:     "sdb",
			UUID:           "edc2348a-a2bc-4b15-b7f3-cb951b3489ad",
			FilesystemType: "btrfs",
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

	blockDeviceStorageInstanceResults := []params.StorageInstanceResult{{
		Result: storage.StorageInstance{
			Id:   "needs-a-filesystem",
			Kind: storage.StorageKindFilesystem,
		},
	}, {
		Result: storage.StorageInstance{
			Id:   "already-has-a-filesystem",
			Kind: storage.StorageKindFilesystem,
		},
	}, {
		Result: storage.StorageInstance{
			Id:   "doesnt-need-a-filesystem",
			Kind: storage.StorageKindBlock,
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeNotAssigned,
			Message: "3 is not assigned to a storage instance",
		},
	}}

	blockDeviceAttachedResults := []params.BoolResult{
		{Result: true},
		{Result: true},
		{Result: true},
		{Result: true},
		{Result: false},
		{Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "5 not found",
		}},
	}

	accessor := &mockBlockDeviceAccessor{
		changes: make(chan []string),
		blockDevice: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			expect := make([]names.DiskTag, len(ids))
			for i, id := range ids {
				expect[i] = names.NewDiskTag(id)
			}
			c.Assert(tags, gc.DeepEquals, expect)
			return params.BlockDeviceResults{blockDeviceResults}, nil
		},
		blockDeviceAttached: func(tags []names.DiskTag) (params.BoolResults, error) {
			return params.BoolResults{blockDeviceAttachedResults}, nil
		},
		blockDeviceStorageInstance: func(tags []names.DiskTag) (params.StorageInstanceResults, error) {
			expect := make([]names.DiskTag, 0, len(blockDeviceResults))
			for i, result := range blockDeviceAttachedResults {
				if !result.Result {
					continue
				}
				expect = append(expect, names.NewDiskTag(ids[i]))
			}
			c.Assert(tags, gc.DeepEquals, expect)
			return params.StorageInstanceResults{blockDeviceStorageInstanceResults}, nil
		},
	}

	testing.PatchExecutableAsEchoArgs(c, s, "mkfs.ext4")

	w := diskformatter.NewWorker(accessor)
	accessor.changes <- ids
	done := make(chan struct{})
	go func() {
		w.Kill()
		w.Wait()
		close(done)
	}()

	select {
	case <-done:
		testing.AssertEchoArgs(c, "mkfs.ext4", "/dev/disk/by-label/dev0-label")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for diskformatter to update")
	}
}

func (s *DiskFormatterWorkerSuite) TestMakeDefaultFilesystem(c *gc.C) {
	accessor := &mockBlockDeviceAccessor{
		blockDevice: func([]names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{
					Name:       "0",
					DeviceName: "sda",
					Label:      "dev0-label",
					UUID:       "9aade75c-6528-4acf-ab69-258b8dc51798",
				},
			}}}, nil
		},
		blockDeviceAttached: func(tags []names.DiskTag) (params.BoolResults, error) {
			return params.BoolResults{[]params.BoolResult{{Result: true}}}, nil
		},
		blockDeviceStorageInstance: func(tags []names.DiskTag) (params.StorageInstanceResults, error) {
			return params.StorageInstanceResults{[]params.StorageInstanceResult{{
				Result: storage.StorageInstance{
					Id:   "needs-a-filesystem",
					Kind: storage.StorageKindFilesystem,
				},
			}}}, nil
		},
	}

	testing.PatchExecutableAsEchoArgs(c, s, "mkfs.ext4")
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.IsNil)
	testing.AssertEchoArgs(c, "mkfs.ext4", "/dev/disk/by-label/dev0-label")
}

func (s *DiskFormatterWorkerSuite) TestBlockDeviceError(c *gc.C) {
	accessor := &mockBlockDeviceAccessor{
		blockDevice: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{}, errors.New("BlockDevice failed")
		},
	}
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.ErrorMatches, "cannot get block devices: BlockDevice failed")
}

func (s *DiskFormatterWorkerSuite) TestBlockDeviceStorageInstanceError(c *gc.C) {
	accessor := &mockBlockDeviceAccessor{
		blockDevice: func([]names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Name: "0", DeviceName: "sda"},
			}}}, nil
		},
		blockDeviceAttached: func(tags []names.DiskTag) (params.BoolResults, error) {
			return params.BoolResults{[]params.BoolResult{{Result: true}}}, nil
		},
		blockDeviceStorageInstance: func(tags []names.DiskTag) (params.StorageInstanceResults, error) {
			return params.StorageInstanceResults{}, errors.New("BlockDeviceStorageInstance failed")
		},
	}
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.ErrorMatches, "cannot get assigned storage instances: BlockDeviceStorageInstance failed")
}

func (s *DiskFormatterWorkerSuite) TestBlockDeviceAttachedError(c *gc.C) {
	accessor := &mockBlockDeviceAccessor{
		blockDevice: func([]names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Name: "0", DeviceName: "sda"},
			}}}, nil
		},
		blockDeviceAttached: func(tags []names.DiskTag) (params.BoolResults, error) {
			return params.BoolResults{}, errors.New("BlockDeviceAttached failed")
		},
		blockDeviceStorageInstance: func(tags []names.DiskTag) (params.StorageInstanceResults, error) {
			return params.StorageInstanceResults{[]params.StorageInstanceResult{{
				Result: storage.StorageInstance{
					Id:   "needs-a-filesystem",
					Kind: storage.StorageKindFilesystem,
				},
			}}}, nil
		},
	}
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.ErrorMatches, "cannot get block device attachment status: BlockDeviceAttached failed")
}

func (s *DiskFormatterWorkerSuite) TestCannotMakeFilesystem(c *gc.C) {
	accessor := &mockBlockDeviceAccessor{
		blockDevice: func(tags []names.DiskTag) (params.BlockDeviceResults, error) {
			return params.BlockDeviceResults{[]params.BlockDeviceResult{{
				Result: storage.BlockDevice{Name: "0", DeviceName: "sda"},
			}}}, nil
		},
		blockDeviceAttached: func(tags []names.DiskTag) (params.BoolResults, error) {
			return params.BoolResults{[]params.BoolResult{{Result: true}}}, nil
		},
		blockDeviceStorageInstance: func(tags []names.DiskTag) (params.StorageInstanceResults, error) {
			return params.StorageInstanceResults{[]params.StorageInstanceResult{{
				Result: storage.StorageInstance{
					Id:   "needs-a-filesystem",
					Kind: storage.StorageKindFilesystem,
				},
			}}}, nil
		},
	}
	// Failure to create a filesystem should not cause the handler to error.
	testing.PatchExecutableThrowError(c, s, "mkfs.ext4", 1)
	formatter := diskformatter.NewDiskFormatter(accessor)
	err := formatter.Handle([]string{"0"})
	c.Assert(err, gc.IsNil)
}

type mockBlockDeviceAccessor struct {
	changes                    chan []string
	blockDevice                func([]names.DiskTag) (params.BlockDeviceResults, error)
	blockDeviceAttached        func([]names.DiskTag) (params.BoolResults, error)
	blockDeviceStorageInstance func([]names.DiskTag) (params.StorageInstanceResults, error)
}

func (m *mockBlockDeviceAccessor) Changes() <-chan []string {
	return m.changes
}

func (m *mockBlockDeviceAccessor) Stop() error {
	return nil
}

func (m *mockBlockDeviceAccessor) Err() error {
	return nil
}

func (m *mockBlockDeviceAccessor) WatchBlockDevices() (watcher.StringsWatcher, error) {
	return m, nil
}

func (m *mockBlockDeviceAccessor) BlockDevice(tags []names.DiskTag) (params.BlockDeviceResults, error) {
	return m.blockDevice(tags)
}

func (m *mockBlockDeviceAccessor) BlockDeviceAttached(tags []names.DiskTag) (params.BoolResults, error) {
	return m.blockDeviceAttached(tags)
}

func (m *mockBlockDeviceAccessor) BlockDeviceStorageInstance(tags []names.DiskTag) (params.StorageInstanceResults, error) {
	return m.blockDeviceStorageInstance(tags)
}
