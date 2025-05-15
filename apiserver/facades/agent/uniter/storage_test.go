// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&storageSuite{})

func (s *storageSuite) TestWatchUnitStorageAttachments(c *tc.C) {
	resources := common.NewResources()
	getCanAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	watcher := &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
	watcher.changes <- []string{"storage/0", "storage/1"}
	st := &mockStorageState{
		watchStorageAttachments: func(u names.UnitTag) state.StringsWatcher {
			c.Assert(u, tc.DeepEquals, unitTag)
			return watcher
		},
	}
	blockDeviceService := &mockBlockDeviceService{}

	storage, err := uniter.NewStorageAPI(st, st, blockDeviceService, resources, getCanAccess)
	c.Assert(err, tc.ErrorIsNil)
	watches, err := storage.WatchUnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: unitTag.String()}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{"storage/0", "storage/1"},
		}},
	})
	c.Assert(resources.Get("1"), tc.Equals, watcher)
}

func (s *storageSuite) TestWatchStorageAttachmentVolume(c *tc.C) {
	resources := common.NewResources()
	getCanAccess := func(ctx context.Context) (common.AuthFunc, error) {
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
	storageWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	storageWatcher.changes <- struct{}{}
	volumeWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	volumeWatcher.changes <- struct{}{}
	blockDevicesWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	blockDevicesWatcher.changes <- struct{}{}
	var calls []string
	st := &mockStorageState{
		assignedMachine: "66",
		storageInstance: func(s names.StorageTag) (state.StorageInstance, error) {
			calls = append(calls, "StorageInstance")
			c.Assert(s, tc.DeepEquals, storageTag)
			return storageInstance, nil
		},
		storageInstanceVolume: func(s names.StorageTag) (state.Volume, error) {
			calls = append(calls, "StorageInstanceVolume")
			c.Assert(s, tc.DeepEquals, storageTag)
			return volume, nil
		},
		watchStorageAttachment: func(s names.StorageTag, u names.UnitTag) state.NotifyWatcher {
			calls = append(calls, "WatchStorageAttachment")
			c.Assert(s, tc.DeepEquals, storageTag)
			c.Assert(u, tc.DeepEquals, unitTag)
			return storageWatcher
		},
		watchVolumeAttachment: func(host names.Tag, v names.VolumeTag) state.NotifyWatcher {
			calls = append(calls, "WatchVolumeAttachment")
			c.Assert(host, tc.DeepEquals, machineTag)
			c.Assert(v, tc.DeepEquals, volumeTag)
			return volumeWatcher
		},
	}
	blockDeviceService := &mockBlockDeviceService{
		watchBlockDevices: func(machineId string) watcher.NotifyWatcher {
			calls = append(calls, "WatchBlockDevices")
			c.Assert(machineId, tc.DeepEquals, machineTag.Id())
			return blockDevicesWatcher
		},
	}

	storage, err := uniter.NewStorageAPI(st, st, blockDeviceService, resources, getCanAccess)
	c.Assert(err, tc.ErrorIsNil)
	watches, err := storage.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{{
			StorageTag: storageTag.String(),
			UnitTag:    unitTag.String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	c.Assert(calls, tc.DeepEquals, []string{
		"StorageInstance",
		"StorageInstanceVolume",
		"WatchVolumeAttachment",
		"WatchBlockDevices",
		"WatchStorageAttachment",
	})
}

func (s *storageSuite) TestCAASWatchStorageAttachmentFilesystem(c *tc.C) {
	s.assertWatchStorageAttachmentFilesystem(c, "")
}

func (s *storageSuite) TestIAASWatchStorageAttachmentFilesystem(c *tc.C) {
	s.assertWatchStorageAttachmentFilesystem(c, "66")
}

func (s *storageSuite) assertWatchStorageAttachmentFilesystem(c *tc.C, assignedMachine string) {
	resources := common.NewResources()
	getCanAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	storageTag := names.NewStorageTag("data/0")
	var hostTag names.Tag
	hostTag = unitTag
	if assignedMachine != "" {
		hostTag = names.NewMachineTag(assignedMachine)
	}
	filesystemTag := names.NewFilesystemTag("104")
	filesystem := &mockFilesystem{tag: filesystemTag}
	storageInstance := &mockStorageInstance{kind: state.StorageKindFilesystem}
	storageWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	storageWatcher.changes <- struct{}{}
	filesystemWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	filesystemWatcher.changes <- struct{}{}
	var calls []string
	st := &mockStorageState{
		assignedMachine: assignedMachine,
		storageInstance: func(s names.StorageTag) (state.StorageInstance, error) {
			calls = append(calls, "StorageInstance")
			c.Assert(s, tc.DeepEquals, storageTag)
			return storageInstance, nil
		},
		storageInstanceFilesystem: func(s names.StorageTag) (state.Filesystem, error) {
			calls = append(calls, "StorageInstanceFilesystem")
			c.Assert(s, tc.DeepEquals, storageTag)
			return filesystem, nil
		},
		watchStorageAttachment: func(s names.StorageTag, u names.UnitTag) state.NotifyWatcher {
			calls = append(calls, "WatchStorageAttachment")
			c.Assert(s, tc.DeepEquals, storageTag)
			c.Assert(u, tc.DeepEquals, unitTag)
			return storageWatcher
		},
		watchFilesystemAttachment: func(host names.Tag, f names.FilesystemTag) state.NotifyWatcher {
			calls = append(calls, "WatchFilesystemAttachment")
			c.Assert(host, tc.DeepEquals, hostTag)
			c.Assert(f, tc.DeepEquals, filesystemTag)
			return filesystemWatcher
		},
	}
	blockDeviceService := &mockBlockDeviceService{}

	storage, err := uniter.NewStorageAPI(st, st, blockDeviceService, resources, getCanAccess)
	c.Assert(err, tc.ErrorIsNil)
	watches, err := storage.WatchStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{{
			StorageTag: storageTag.String(),
			UnitTag:    unitTag.String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(watches, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	c.Assert(calls, tc.DeepEquals, []string{
		"StorageInstance",
		"StorageInstanceFilesystem",
		"WatchFilesystemAttachment",
		"WatchStorageAttachment",
	})
}

func (s *storageSuite) TestDestroyUnitStorageAttachments(c *tc.C) {
	resources := common.NewResources()
	getCanAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	var calls []string
	st := &mockStorageState{
		destroyUnitStorageAttachments: func(u names.UnitTag) error {
			calls = append(calls, "DestroyUnitStorageAttachments")
			c.Assert(u, tc.DeepEquals, unitTag)
			return nil
		},
	}
	blockDeviceService := &mockBlockDeviceService{}

	storage, err := uniter.NewStorageAPI(st, st, blockDeviceService, resources, getCanAccess)
	c.Assert(err, tc.ErrorIsNil)
	destroyErrors, err := storage.DestroyUnitStorageAttachments(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: unitTag.String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.DeepEquals, []string{"DestroyUnitStorageAttachments"})
	c.Assert(destroyErrors, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *storageSuite) TestRemoveStorageAttachments(c *tc.C) {
	setMock := func(st *mockStorageState, f func(s names.StorageTag, u names.UnitTag, force bool) error) {
		st.remove = f
	}

	unitTag0 := names.NewUnitTag("mysql/0")
	unitTag1 := names.NewUnitTag("mysql/1")
	storageTag0 := names.NewStorageTag("data/0")
	storageTag1 := names.NewStorageTag("data/1")

	resources := common.NewResources()
	getCanAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == unitTag0
		}, nil
	}

	st := &mockStorageState{}
	setMock(st, func(s names.StorageTag, u names.UnitTag, force bool) error {
		c.Assert(u, tc.DeepEquals, unitTag0)
		if s == storageTag1 {
			return errors.New("badness")
		}
		return nil
	})
	blockDeviceService := &mockBlockDeviceService{}

	storage, err := uniter.NewStorageAPI(st, st, blockDeviceService, resources, getCanAccess)
	c.Assert(err, tc.ErrorIsNil)
	removeErrors, err := storage.RemoveStorageAttachments(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{{
			StorageTag: storageTag0.String(),
			UnitTag:    unitTag0.String(),
		}, {
			StorageTag: storageTag1.String(),
			UnitTag:    unitTag0.String(),
		}, {
			StorageTag: storageTag0.String(),
			UnitTag:    unitTag1.String(),
		}, {
			StorageTag: unitTag0.String(), // oops
			UnitTag:    unitTag0.String(),
		}, {
			StorageTag: storageTag0.String(),
			UnitTag:    storageTag0.String(), // oops
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removeErrors, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: &params.Error{Message: "badness"}},
			{Error: &params.Error{Code: params.CodeUnauthorized, Message: "permission denied"}},
			{Error: &params.Error{Message: `"unit-mysql-0" is not a valid storage tag`}},
			{Error: &params.Error{Message: `"storage-data-0" is not a valid unit tag`}},
		},
	})
}

type mockUnit struct {
	assignedMachine    string
	storageConstraints map[string]state.StorageConstraints
}

func (u *mockUnit) ShouldBeAssigned() bool {
	return u.assignedMachine != ""
}

func (u *mockUnit) AssignedMachineId() (string, error) {
	if u.assignedMachine == "" {
		return "", errors.NotAssignedf("unit not assigned")
	}
	return u.assignedMachine, nil
}

func (u *mockUnit) StorageConstraints() (map[string]state.StorageConstraints, error) {
	return u.storageConstraints, nil
}

type mockBlockDeviceService struct {
	uniter.BlockDeviceService
	watchBlockDevices func(string) watcher.NotifyWatcher
}

func (m *mockBlockDeviceService) WatchBlockDevices(_ context.Context, machineId string) (watcher.NotifyWatcher, error) {
	return m.watchBlockDevices(machineId), nil
}

type mockStorageState struct {
	unitStorageConstraints map[string]state.StorageConstraints
	assignedMachine        string

	uniter.Backend
	uniter.StorageStateInterface
	uniter.StorageVolumeInterface
	uniter.StorageFilesystemInterface
	destroyUnitStorageAttachments func(names.UnitTag) error
	remove                        func(names.StorageTag, names.UnitTag, bool) error
	storageInstance               func(names.StorageTag) (state.StorageInstance, error)
	storageInstanceFilesystem     func(names.StorageTag) (state.Filesystem, error)
	storageInstanceVolume         func(names.StorageTag) (state.Volume, error)
	watchStorageAttachments       func(names.UnitTag) state.StringsWatcher
	watchStorageAttachment        func(names.StorageTag, names.UnitTag) state.NotifyWatcher
	watchFilesystemAttachment     func(names.Tag, names.FilesystemTag) state.NotifyWatcher
	watchVolumeAttachment         func(names.Tag, names.VolumeTag) state.NotifyWatcher
	addUnitStorageOperation       func(u names.UnitTag, name string, cons state.StorageConstraints) error
}

func (m *mockStorageState) VolumeAccess() uniter.StorageVolumeInterface {
	return m
}

func (m *mockStorageState) FilesystemAccess() uniter.StorageFilesystemInterface {
	return m
}

func (m *mockStorageState) Unit(name string) (uniter.Unit, error) {
	return &mockUnit{
		assignedMachine:    m.assignedMachine,
		storageConstraints: m.unitStorageConstraints}, nil
}

func (m *mockStorageState) DestroyUnitStorageAttachments(u names.UnitTag) error {
	return m.destroyUnitStorageAttachments(u)
}

func (m *mockStorageState) RemoveStorageAttachment(s names.StorageTag, u names.UnitTag, force bool) error {
	return m.remove(s, u, force)
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

func (m *mockStorageState) WatchStorageAttachments(u names.UnitTag) state.StringsWatcher {
	return m.watchStorageAttachments(u)
}

func (m *mockStorageState) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) state.NotifyWatcher {
	return m.watchStorageAttachment(s, u)
}

func (m *mockStorageState) WatchFilesystemAttachment(hostTag names.Tag, f names.FilesystemTag) state.NotifyWatcher {
	return m.watchFilesystemAttachment(hostTag, f)
}

func (m *mockStorageState) WatchVolumeAttachment(hostTag names.Tag, v names.VolumeTag) state.NotifyWatcher {
	return m.watchVolumeAttachment(hostTag, v)
}

func (m *mockStorageState) AddStorageForUnitOperation(tag names.UnitTag, name string, cons state.StorageConstraints) (state.ModelOperation, error) {
	return nil, m.addUnitStorageOperation(tag, name, cons)
}

func (m *mockStorageState) ApplyOperation(state.ModelOperation) error {
	return nil
}

type mockStringsWatcher struct {
	state.StringsWatcher
	changes chan []string
}

func (m *mockStringsWatcher) Changes() <-chan []string {
	return m.changes
}

type mockNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func (m *mockNotifyWatcher) Stop() error {
	m.Kill()
	return m.Wait()
}

func (m *mockNotifyWatcher) Kill() {
	m.tomb.Kill(nil)
}

func (m *mockNotifyWatcher) Wait() error {
	return m.tomb.Wait()
}

func (m *mockNotifyWatcher) Err() error {
	return m.tomb.Err()
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

type watchStorageAttachmentSuite struct {
	storageTag               names.StorageTag
	machineTag               names.MachineTag
	unitTag                  names.UnitTag
	st                       *fakeStorage
	storageInstance          *fakeStorageInstance
	volume                   *fakeVolume
	blockDeviceService       *fakeBlockDevices
	volumeAttachmentWatcher  *apiservertesting.FakeNotifyWatcher
	blockDevicesWatcher      *apiservertesting.FakeNotifyWatcher
	storageAttachmentWatcher *apiservertesting.FakeNotifyWatcher
}

var _ = tc.Suite(&watchStorageAttachmentSuite{})

func (s *watchStorageAttachmentSuite) SetUpTest(c *tc.C) {
	s.storageTag = names.NewStorageTag("osd-devices/0")
	s.machineTag = names.NewMachineTag("0")
	s.unitTag = names.NewUnitTag("ceph/0")
	s.storageInstance = &fakeStorageInstance{
		tag:   s.storageTag,
		owner: s.machineTag,
		kind:  state.StorageKindBlock,
	}
	s.volume = &fakeVolume{tag: names.NewVolumeTag("0")}
	s.volumeAttachmentWatcher = apiservertesting.NewFakeNotifyWatcher()
	s.blockDevicesWatcher = apiservertesting.NewFakeNotifyWatcher()
	s.storageAttachmentWatcher = apiservertesting.NewFakeNotifyWatcher()
	s.st = &fakeStorage{
		storageInstance: func(tag names.StorageTag) (state.StorageInstance, error) {
			return s.storageInstance, nil
		},
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return s.volume, nil
		},
		watchVolumeAttachment: func(names.Tag, names.VolumeTag) state.NotifyWatcher {
			return s.volumeAttachmentWatcher
		},
		watchStorageAttachment: func(names.StorageTag, names.UnitTag) state.NotifyWatcher {
			return s.storageAttachmentWatcher
		},
	}
	s.blockDeviceService = &fakeBlockDevices{
		watchBlockDevices: func(string) watcher.NotifyWatcher {
			return s.blockDevicesWatcher
		},
	}
}

func (s *watchStorageAttachmentSuite) TestWatchStorageAttachmentVolumeAttachmentChanges(c *tc.C) {
	s.testWatchBlockStorageAttachment(c, func() {
		s.volumeAttachmentWatcher.C <- struct{}{}
	})
}

func (s *watchStorageAttachmentSuite) TestWatchStorageAttachmentStorageAttachmentChanges(c *tc.C) {
	s.testWatchBlockStorageAttachment(c, func() {
		s.storageAttachmentWatcher.C <- struct{}{}
	})
}

func (s *watchStorageAttachmentSuite) TestWatchStorageAttachmentBlockDevicesChange(c *tc.C) {
	s.testWatchBlockStorageAttachment(c, func() {
		s.blockDevicesWatcher.C <- struct{}{}
	})
}

func (s *watchStorageAttachmentSuite) testWatchBlockStorageAttachment(c *tc.C, change func()) {
	s.testWatchStorageAttachment(c, change)
	s.st.CheckCallNames(c,
		"StorageInstance",
		"StorageInstanceVolume",
		"WatchVolumeAttachment",
		"WatchStorageAttachment",
	)
	s.blockDeviceService.CheckCallNames(c, "WatchBlockDevices")
}

func (s *watchStorageAttachmentSuite) testWatchStorageAttachment(c *tc.C, change func()) {
	w, err := uniter.WatchStorageAttachment(
		c.Context(),
		s.st,
		s.st,
		s.st,
		s.blockDeviceService,
		s.storageTag,
		s.machineTag,
		s.unitTag,
	)
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()
	change()
	wc.AssertOneChange()
}
