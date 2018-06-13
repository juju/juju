// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/state"
)

type fakeStorage struct {
	testing.Stub
	uniter.StorageStateInterface
	uniter.StorageFilesystemInterface
	storageInstance        func(names.StorageTag) (state.StorageInstance, error)
	storageInstanceVolume  func(names.StorageTag) (state.Volume, error)
	volumeAttachment       func(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	blockDevices           func(names.MachineTag) ([]state.BlockDeviceInfo, error)
	watchVolumeAttachment  func(names.MachineTag, names.VolumeTag) state.NotifyWatcher
	watchBlockDevices      func(names.MachineTag) state.NotifyWatcher
	watchStorageAttachment func(names.StorageTag, names.UnitTag) state.NotifyWatcher
}

func (s *fakeStorage) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	s.MethodCall(s, "StorageInstance", tag)
	return s.storageInstance(tag)
}

func (s *fakeStorage) StorageInstanceVolume(tag names.StorageTag) (state.Volume, error) {
	s.MethodCall(s, "StorageInstanceVolume", tag)
	return s.storageInstanceVolume(tag)
}

func (s *fakeStorage) VolumeAttachment(m names.MachineTag, v names.VolumeTag) (state.VolumeAttachment, error) {
	s.MethodCall(s, "VolumeAttachment", m, v)
	return s.volumeAttachment(m, v)
}

func (s *fakeStorage) BlockDevices(m names.MachineTag) ([]state.BlockDeviceInfo, error) {
	s.MethodCall(s, "BlockDevices", m)
	return s.blockDevices(m)
}

func (s *fakeStorage) WatchVolumeAttachment(m names.MachineTag, v names.VolumeTag) state.NotifyWatcher {
	s.MethodCall(s, "WatchVolumeAttachment", m, v)
	return s.watchVolumeAttachment(m, v)
}

func (s *fakeStorage) WatchBlockDevices(m names.MachineTag) state.NotifyWatcher {
	s.MethodCall(s, "WatchBlockDevices", m)
	return s.watchBlockDevices(m)
}

func (s *fakeStorage) WatchStorageAttachment(st names.StorageTag, u names.UnitTag) state.NotifyWatcher {
	s.MethodCall(s, "WatchStorageAttachment", st, u)
	return s.watchStorageAttachment(st, u)
}

type fakeStorageInstance struct {
	state.StorageInstance
	tag   names.StorageTag
	owner names.Tag
	kind  state.StorageKind
}

func (i *fakeStorageInstance) StorageTag() names.StorageTag {
	return i.tag
}

func (i *fakeStorageInstance) Tag() names.Tag {
	return i.tag
}

func (i *fakeStorageInstance) Owner() (names.Tag, bool) {
	return i.owner, i.owner != nil
}

func (i *fakeStorageInstance) Kind() state.StorageKind {
	return i.kind
}

type fakeVolume struct {
	state.Volume
	tag    names.VolumeTag
	params *state.VolumeParams
	info   *state.VolumeInfo
}

func (v *fakeVolume) VolumeTag() names.VolumeTag {
	return v.tag
}

func (v *fakeVolume) Tag() names.Tag {
	return v.tag
}

func (v *fakeVolume) Params() (state.VolumeParams, bool) {
	if v.params == nil {
		return state.VolumeParams{}, false
	}
	return *v.params, true
}

func (v *fakeVolume) Info() (state.VolumeInfo, error) {
	if v.info == nil {
		return state.VolumeInfo{}, errors.NotProvisionedf("volume %v", v.tag.Id())
	}
	return *v.info, nil
}

type nopSyncStarter struct{}

func (nopSyncStarter) StartSync() {}
