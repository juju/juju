// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestWatchUnitStorageAttachments(c *gc.C) {
	resources := common.NewResources()
	getCanAccess := func() (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	watcher := &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
	watcher.changes <- []string{"storage/0", "storage/1"}
	state := &mockStorageState{
		watchStorageAttachments: func(u names.UnitTag) state.StringsWatcher {
			c.Assert(u, gc.DeepEquals, unitTag)
			return watcher
		},
	}

	storage, err := uniter.NewStorageAPI(state, resources, getCanAccess)
	c.Assert(err, jc.ErrorIsNil)
	watches, err := storage.WatchUnitStorageAttachments(params.Entities{
		Entities: []params.Entity{{unitTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{"storage/0", "storage/1"},
		}},
	})
	c.Assert(resources.Get("1"), gc.Equals, watcher)
}

func (s *storageSuite) TestWatchStorageAttachmentVolume(c *gc.C) {
	resources := common.NewResources()
	getCanAccess := func() (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	storageTag := names.NewStorageTag("data/0")
	machineTag := names.NewMachineTag("66")
	volumeTag := names.NewVolumeTag("104")
	volume := &mockVolume{tag: volumeTag}
	storageInstance := &mockStorageInstance{kind: state.StorageKindBlock}
	watcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	watcher.changes <- struct{}{}
	var calls []string
	state := &mockStorageState{
		storageInstance: func(s names.StorageTag) (state.StorageInstance, error) {
			calls = append(calls, "StorageInstance")
			c.Assert(s, gc.DeepEquals, storageTag)
			return storageInstance, nil
		},
		storageInstanceVolume: func(s names.StorageTag) (state.Volume, error) {
			calls = append(calls, "StorageInstanceVolume")
			c.Assert(s, gc.DeepEquals, storageTag)
			return volume, nil
		},
		unitAssignedMachine: func(u names.UnitTag) (names.MachineTag, error) {
			calls = append(calls, "UnitAssignedMachine")
			c.Assert(u, gc.DeepEquals, unitTag)
			return machineTag, nil
		},
		watchVolumeAttachment: func(m names.MachineTag, v names.VolumeTag) state.NotifyWatcher {
			calls = append(calls, "WatchVolumeAttachment")
			c.Assert(m, gc.DeepEquals, machineTag)
			c.Assert(v, gc.DeepEquals, volumeTag)
			return watcher
		},
	}

	storage, err := uniter.NewStorageAPI(state, resources, getCanAccess)
	c.Assert(err, jc.ErrorIsNil)
	watches, err := storage.WatchStorageAttachmentInfos(params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{{
			StorageTag: storageTag.String(),
			UnitTag:    unitTag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	c.Assert(resources.Get("1"), gc.Equals, watcher)
	c.Assert(calls, gc.DeepEquals, []string{
		"UnitAssignedMachine",
		"StorageInstance",
		"StorageInstanceVolume",
		"WatchVolumeAttachment",
	})
}

func (s *storageSuite) TestWatchStorageAttachmentFilesystem(c *gc.C) {
	resources := common.NewResources()
	getCanAccess := func() (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	storageTag := names.NewStorageTag("data/0")
	machineTag := names.NewMachineTag("66")
	filesystemTag := names.NewFilesystemTag("104")
	filesystem := &mockFilesystem{tag: filesystemTag}
	storageInstance := &mockStorageInstance{kind: state.StorageKindFilesystem}
	watcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	watcher.changes <- struct{}{}
	var calls []string
	state := &mockStorageState{
		storageInstance: func(s names.StorageTag) (state.StorageInstance, error) {
			calls = append(calls, "StorageInstance")
			c.Assert(s, gc.DeepEquals, storageTag)
			return storageInstance, nil
		},
		storageInstanceFilesystem: func(s names.StorageTag) (state.Filesystem, error) {
			calls = append(calls, "StorageInstanceFilesystem")
			c.Assert(s, gc.DeepEquals, storageTag)
			return filesystem, nil
		},
		unitAssignedMachine: func(u names.UnitTag) (names.MachineTag, error) {
			calls = append(calls, "UnitAssignedMachine")
			c.Assert(u, gc.DeepEquals, unitTag)
			return machineTag, nil
		},
		watchFilesystemAttachment: func(m names.MachineTag, f names.FilesystemTag) state.NotifyWatcher {
			calls = append(calls, "WatchFilesystemAttachment")
			c.Assert(m, gc.DeepEquals, machineTag)
			c.Assert(f, gc.DeepEquals, filesystemTag)
			return watcher
		},
	}

	storage, err := uniter.NewStorageAPI(state, resources, getCanAccess)
	c.Assert(err, jc.ErrorIsNil)
	watches, err := storage.WatchStorageAttachmentInfos(params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{{
			StorageTag: storageTag.String(),
			UnitTag:    unitTag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	c.Assert(resources.Get("1"), gc.Equals, watcher)
	c.Assert(calls, gc.DeepEquals, []string{
		"UnitAssignedMachine",
		"StorageInstance",
		"StorageInstanceFilesystem",
		"WatchFilesystemAttachment",
	})
}

type mockStorageState struct {
	uniter.StorageStateInterface
	storageInstance           func(names.StorageTag) (state.StorageInstance, error)
	storageInstanceFilesystem func(names.StorageTag) (state.Filesystem, error)
	storageInstanceVolume     func(names.StorageTag) (state.Volume, error)
	unitAssignedMachine       func(names.UnitTag) (names.MachineTag, error)
	watchStorageAttachments   func(names.UnitTag) state.StringsWatcher
	watchFilesystemAttachment func(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
	watchVolumeAttachment     func(names.MachineTag, names.VolumeTag) state.NotifyWatcher
}

func (m *mockStorageState) StorageInstance(s names.StorageTag) (state.StorageInstance, error) {
	return m.storageInstance(s)
}

func (m *mockStorageState) StorageInstanceFilesystem(s names.StorageTag) (state.Filesystem, error) {
	return m.storageInstanceFilesystem(s)
}

func (m *mockStorageState) StorageInstanceVolume(s names.StorageTag) (state.Volume, error) {
	return m.storageInstanceVolume(s)
}

func (m *mockStorageState) UnitAssignedMachine(u names.UnitTag) (names.MachineTag, error) {
	return m.unitAssignedMachine(u)
}

func (m *mockStorageState) WatchStorageAttachments(u names.UnitTag) state.StringsWatcher {
	return m.watchStorageAttachments(u)
}

func (m *mockStorageState) WatchFilesystemAttachment(mtag names.MachineTag, f names.FilesystemTag) state.NotifyWatcher {
	return m.watchFilesystemAttachment(mtag, f)
}

func (m *mockStorageState) WatchVolumeAttachment(mtag names.MachineTag, v names.VolumeTag) state.NotifyWatcher {
	return m.watchVolumeAttachment(mtag, v)
}

type mockStringsWatcher struct {
	state.StringsWatcher
	changes chan []string
}

func (m *mockStringsWatcher) Changes() <-chan []string {
	return m.changes
}

type mockNotifyWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (m *mockNotifyWatcher) Changes() <-chan struct{} {
	return m.changes
}

type mockVolume struct {
	state.Volume
	tag names.VolumeTag
}

func (m *mockVolume) VolumeTag() names.VolumeTag {
	return m.tag
}

type mockFilesystem struct {
	state.Filesystem
	tag names.FilesystemTag
}

func (m *mockFilesystem) FilesystemTag() names.FilesystemTag {
	return m.tag
}

type mockStorageInstance struct {
	state.StorageInstance
	kind state.StorageKind
}

func (m *mockStorageInstance) Kind() state.StorageKind {
	return m.kind
}
