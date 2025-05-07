// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type fakeBlockDevices struct {
	testhelpers.Stub
	blockDevices      func(string) ([]blockdevice.BlockDevice, error)
	watchBlockDevices func(string) watcher.NotifyWatcher
}

func (s *fakeBlockDevices) BlockDevices(_ context.Context, machineId string) ([]blockdevice.BlockDevice, error) {
	s.MethodCall(s, "BlockDevices", machineId)
	return s.blockDevices(machineId)
}

func (s *fakeBlockDevices) WatchBlockDevices(_ context.Context, machineId string) (watcher.NotifyWatcher, error) {
	s.MethodCall(s, "WatchBlockDevices", machineId)
	return s.watchBlockDevices(machineId), nil
}

type fakeStorage struct {
	testhelpers.Stub
	uniter.StorageStateInterface
	uniter.StorageFilesystemInterface
	storageInstance        func(names.StorageTag) (state.StorageInstance, error)
	storageInstanceVolume  func(names.StorageTag) (state.Volume, error)
	volumeAttachment       func(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	volumeAttachmentPlan   func(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
	watchVolumeAttachment  func(names.Tag, names.VolumeTag) state.NotifyWatcher
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

func (s *fakeStorage) VolumeAttachment(m names.Tag, v names.VolumeTag) (state.VolumeAttachment, error) {
	s.MethodCall(s, "VolumeAttachment", m, v)
	return s.volumeAttachment(m, v)
}

func (s *fakeStorage) VolumeAttachmentPlan(host names.Tag, volume names.VolumeTag) (state.VolumeAttachmentPlan, error) {
	s.MethodCall(s, "VolumeAttachmentPlan", host, volume)
	return s.volumeAttachmentPlan(host, volume)
}

func (s *fakeStorage) WatchVolumeAttachment(host names.Tag, v names.VolumeTag) state.NotifyWatcher {
	s.MethodCall(s, "WatchVolumeAttachment", host, v)
	return s.watchVolumeAttachment(host, v)
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
