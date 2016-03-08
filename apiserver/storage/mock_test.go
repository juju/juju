// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
	jujustorage "github.com/juju/juju/storage"
)

type mockPoolManager struct {
	getPool    func(name string) (*jujustorage.Config, error)
	createPool func(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error)
	deletePool func(name string) error
	listPools  func() ([]*jujustorage.Config, error)
}

func (m *mockPoolManager) Get(name string) (*jujustorage.Config, error) {
	return m.getPool(name)
}

func (m *mockPoolManager) Create(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error) {
	return m.createPool(name, providerType, attrs)
}

func (m *mockPoolManager) Delete(name string) error {
	return m.deletePool(name)
}

func (m *mockPoolManager) List() ([]*jujustorage.Config, error) {
	return m.listPools()
}

type mockState struct {
	storageInstance                     func(names.StorageTag) (state.StorageInstance, error)
	allStorageInstances                 func() ([]state.StorageInstance, error)
	storageInstanceAttachments          func(names.StorageTag) ([]state.StorageAttachment, error)
	unitAssignedMachine                 func(u names.UnitTag) (names.MachineTag, error)
	storageInstanceVolume               func(names.StorageTag) (state.Volume, error)
	volumeAttachment                    func(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	storageInstanceFilesystem           func(names.StorageTag) (state.Filesystem, error)
	storageInstanceFilesystemAttachment func(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error)
	watchStorageAttachment              func(names.StorageTag, names.UnitTag) state.NotifyWatcher
	watchFilesystemAttachment           func(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
	watchVolumeAttachment               func(names.MachineTag, names.VolumeTag) state.NotifyWatcher
	watchBlockDevices                   func(names.MachineTag) state.NotifyWatcher
	modelName                           string
	volume                              func(tag names.VolumeTag) (state.Volume, error)
	machineVolumeAttachments            func(machine names.MachineTag) ([]state.VolumeAttachment, error)
	volumeAttachments                   func(volume names.VolumeTag) ([]state.VolumeAttachment, error)
	allVolumes                          func() ([]state.Volume, error)
	filesystem                          func(tag names.FilesystemTag) (state.Filesystem, error)
	machineFilesystemAttachments        func(machine names.MachineTag) ([]state.FilesystemAttachment, error)
	filesystemAttachments               func(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error)
	allFilesystems                      func() ([]state.Filesystem, error)
	addStorageForUnit                   func(u names.UnitTag, name string, cons state.StorageConstraints) error
	getBlockForType                     func(t state.BlockType) (state.Block, bool, error)
	blockDevices                        func(names.MachineTag) ([]state.BlockDeviceInfo, error)
}

func (st *mockState) StorageInstance(s names.StorageTag) (state.StorageInstance, error) {
	return st.storageInstance(s)
}

func (st *mockState) AllStorageInstances() ([]state.StorageInstance, error) {
	return st.allStorageInstances()
}

func (st *mockState) StorageAttachments(tag names.StorageTag) ([]state.StorageAttachment, error) {
	return st.storageInstanceAttachments(tag)
}

func (st *mockState) UnitAssignedMachine(unit names.UnitTag) (names.MachineTag, error) {
	return st.unitAssignedMachine(unit)
}

func (st *mockState) FilesystemAttachment(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error) {
	return st.storageInstanceFilesystemAttachment(m, f)
}

func (st *mockState) StorageInstanceFilesystem(s names.StorageTag) (state.Filesystem, error) {
	return st.storageInstanceFilesystem(s)
}

func (st *mockState) StorageInstanceVolume(s names.StorageTag) (state.Volume, error) {
	return st.storageInstanceVolume(s)
}

func (st *mockState) VolumeAttachment(m names.MachineTag, v names.VolumeTag) (state.VolumeAttachment, error) {
	return st.volumeAttachment(m, v)
}

func (st *mockState) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) state.NotifyWatcher {
	return st.watchStorageAttachment(s, u)
}

func (st *mockState) WatchFilesystemAttachment(mtag names.MachineTag, f names.FilesystemTag) state.NotifyWatcher {
	return st.watchFilesystemAttachment(mtag, f)
}

func (st *mockState) WatchVolumeAttachment(mtag names.MachineTag, v names.VolumeTag) state.NotifyWatcher {
	return st.watchVolumeAttachment(mtag, v)
}

func (st *mockState) WatchBlockDevices(mtag names.MachineTag) state.NotifyWatcher {
	return st.watchBlockDevices(mtag)
}

func (st *mockState) ModelName() (string, error) {
	return st.modelName, nil
}

func (st *mockState) AllVolumes() ([]state.Volume, error) {
	return st.allVolumes()
}

func (st *mockState) VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
	return st.volumeAttachments(volume)
}

func (st *mockState) MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error) {
	return st.machineVolumeAttachments(machine)
}

func (st *mockState) Volume(tag names.VolumeTag) (state.Volume, error) {
	return st.volume(tag)
}

func (st *mockState) AllFilesystems() ([]state.Filesystem, error) {
	return st.allFilesystems()
}

func (st *mockState) FilesystemAttachments(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error) {
	return st.filesystemAttachments(filesystem)
}

func (st *mockState) MachineFilesystemAttachments(machine names.MachineTag) ([]state.FilesystemAttachment, error) {
	return st.machineFilesystemAttachments(machine)
}

func (st *mockState) Filesystem(tag names.FilesystemTag) (state.Filesystem, error) {
	return st.filesystem(tag)
}

func (st *mockState) AddStorageForUnit(u names.UnitTag, name string, cons state.StorageConstraints) error {
	return st.addStorageForUnit(u, name, cons)
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return st.getBlockForType(t)
}

func (st *mockState) BlockDevices(m names.MachineTag) ([]state.BlockDeviceInfo, error) {
	if st.blockDevices != nil {
		return st.blockDevices(m)
	}
	return []state.BlockDeviceInfo{}, nil
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
	tag     names.VolumeTag
	storage *names.StorageTag
	info    *state.VolumeInfo
}

func (m *mockVolume) StorageInstance() (names.StorageTag, error) {
	if m.storage != nil {
		return *m.storage, nil
	}
	return names.StorageTag{}, errors.NewNotAssigned(nil, "error from mock")
}

func (m *mockVolume) VolumeTag() names.VolumeTag {
	return m.tag
}

func (m *mockVolume) Params() (state.VolumeParams, bool) {
	return state.VolumeParams{
		Pool: "loop",
		Size: 1024,
	}, true
}

func (m *mockVolume) Info() (state.VolumeInfo, error) {
	if m.info != nil {
		return *m.info, nil
	}
	return state.VolumeInfo{}, errors.NotProvisionedf("%v", m.tag)
}

func (m *mockVolume) Status() (state.StatusInfo, error) {
	return state.StatusInfo{Status: state.StatusAttached}, nil
}

type mockFilesystem struct {
	state.Filesystem
	tag     names.FilesystemTag
	storage *names.StorageTag
	volume  *names.VolumeTag
	info    *state.FilesystemInfo
}

func (m *mockFilesystem) Storage() (names.StorageTag, error) {
	if m.storage != nil {
		return *m.storage, nil
	}
	return names.StorageTag{}, errors.NewNotAssigned(nil, "error from mock")
}

func (m *mockFilesystem) FilesystemTag() names.FilesystemTag {
	return m.tag
}

func (m *mockFilesystem) Volume() (names.VolumeTag, error) {
	if m.volume != nil {
		return *m.volume, nil
	}
	return names.VolumeTag{}, state.ErrNoBackingVolume
}

func (m *mockFilesystem) Info() (state.FilesystemInfo, error) {
	if m.info != nil {
		return *m.info, nil
	}
	return state.FilesystemInfo{}, errors.NotProvisionedf("filesystem")
}

func (m *mockFilesystem) Status() (state.StatusInfo, error) {
	return state.StatusInfo{Status: state.StatusAttached}, nil
}

type mockFilesystemAttachment struct {
	state.FilesystemAttachment
	filesystem names.FilesystemTag
	machine    names.MachineTag
	info       *state.FilesystemAttachmentInfo
}

func (m *mockFilesystemAttachment) Filesystem() names.FilesystemTag {
	return m.filesystem
}

func (m *mockFilesystemAttachment) Machine() names.MachineTag {
	return m.machine
}

func (m *mockFilesystemAttachment) Info() (state.FilesystemAttachmentInfo, error) {
	if m.info != nil {
		return *m.info, nil
	}
	return state.FilesystemAttachmentInfo{}, errors.NotProvisionedf("filesystem attachment")
}

type mockStorageInstance struct {
	state.StorageInstance
	kind       state.StorageKind
	owner      names.Tag
	storageTag names.Tag
}

func (m *mockStorageInstance) Kind() state.StorageKind {
	return m.kind
}

func (m *mockStorageInstance) Owner() names.Tag {
	return m.owner
}

func (m *mockStorageInstance) Tag() names.Tag {
	return m.storageTag
}

func (m *mockStorageInstance) StorageTag() names.StorageTag {
	return m.storageTag.(names.StorageTag)
}

func (m *mockStorageInstance) CharmURL() *charm.URL {
	panic("not implemented for test")
}

type mockStorageAttachment struct {
	state.StorageAttachment
	storage *mockStorageInstance
}

func (m *mockStorageAttachment) StorageInstance() names.StorageTag {
	return m.storage.Tag().(names.StorageTag)
}

func (m *mockStorageAttachment) Unit() names.UnitTag {
	return m.storage.Owner().(names.UnitTag)
}

type mockVolumeAttachment struct {
	VolumeTag  names.VolumeTag
	MachineTag names.MachineTag
	info       *state.VolumeAttachmentInfo
}

func (va *mockVolumeAttachment) Volume() names.VolumeTag {
	return va.VolumeTag
}

func (va *mockVolumeAttachment) Machine() names.MachineTag {
	return va.MachineTag
}

func (va *mockVolumeAttachment) Life() state.Life {
	panic("not implemented for test")
}

func (va *mockVolumeAttachment) Info() (state.VolumeAttachmentInfo, error) {
	if va.info != nil {
		return *va.info, nil
	}
	return state.VolumeAttachmentInfo{}, errors.NotProvisionedf("volume attachment")
}

func (va *mockVolumeAttachment) Params() (state.VolumeAttachmentParams, bool) {
	panic("not implemented for test")
}

type mockBlock struct {
	state.Block
	t   state.BlockType
	msg string
}

func (b mockBlock) Type() state.BlockType {
	return b.t
}

func (b mockBlock) Message() string {
	return b.msg
}
