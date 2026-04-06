// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/common/storagecommon"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

type fakeStorage struct {
	testing.Stub
	storagecommon.StorageAccess
	storagecommon.FilesystemAccess
	storageInstance           func(names.StorageTag) (state.StorageInstance, error)
	storageAttachments        func(names.StorageTag) ([]state.StorageAttachment, error)
	storageInstanceVolume     func(names.StorageTag) (state.Volume, error)
	storageInstanceFilesystem func(names.StorageTag) (state.Filesystem, error)
	volumeAttachment          func(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	volumeAttachmentPlan      func(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
	filesystemAttachment      func(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)
	blockDevices              func(names.MachineTag) ([]state.BlockDeviceInfo, error)
}

func (s *fakeStorage) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	s.MethodCall(s, "StorageInstance", tag)
	return s.storageInstance(tag)
}

func (s *fakeStorage) StorageAttachments(tag names.StorageTag) ([]state.StorageAttachment, error) {
	s.MethodCall(s, "StorageAttachments", tag)
	if s.storageAttachments == nil {
		return nil, nil
	}
	return s.storageAttachments(tag)
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

func (s *fakeStorage) BlockDevices(m names.MachineTag) ([]state.BlockDeviceInfo, error) {
	s.MethodCall(s, "BlockDevices", m)
	return s.blockDevices(m)
}

func (s *fakeStorage) StorageInstanceFilesystem(tag names.StorageTag) (state.Filesystem, error) {
	s.MethodCall(s, "StorageInstanceFilesystem", tag)
	return s.storageInstanceFilesystem(tag)
}

func (s *fakeStorage) FilesystemAttachment(m names.Tag, fs names.FilesystemTag) (state.FilesystemAttachment, error) {
	s.MethodCall(s, "FilesystemAttachment", m, fs)
	return s.filesystemAttachment(m, fs)
}

type fakeStorageInstance struct {
	state.StorageInstance
	tag   names.StorageTag
	owner names.Tag
	kind  state.StorageKind
	life  state.Life
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

func (i *fakeStorageInstance) Life() state.Life {
	return i.life
}

type fakeStorageAttachment struct {
	state.StorageAttachment
	storageTag names.StorageTag
	unitTag    names.UnitTag
	life       state.Life
}

func (a *fakeStorageAttachment) StorageInstance() names.StorageTag {
	return a.storageTag
}

func (a *fakeStorageAttachment) Unit() names.UnitTag {
	return a.unitTag
}

func (a *fakeStorageAttachment) Life() state.Life {
	return a.life
}

type fakeVolume struct {
	state.Volume
	tag    names.VolumeTag
	params *state.VolumeParams
	info   *state.VolumeInfo
	status *corestatus.StatusInfo
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

func (v *fakeVolume) Status() (corestatus.StatusInfo, error) {
	if v.status == nil {
		return corestatus.StatusInfo{Status: corestatus.Attached}, nil
	}
	return *v.status, nil
}

type fakeVolumeAttachment struct {
	state.VolumeAttachment
	info *state.VolumeAttachmentInfo
}

func (v *fakeVolumeAttachment) Info() (state.VolumeAttachmentInfo, error) {
	if v.info == nil {
		return state.VolumeAttachmentInfo{}, errors.NotProvisionedf("volume attachment")
	}
	return *v.info, nil
}

type fakeVolumeAttachmentPlan struct {
	state.VolumeAttachmentPlan
	blockInfo *state.BlockDeviceInfo
	err       error
}

func (p *fakeVolumeAttachmentPlan) BlockDeviceInfo() (state.BlockDeviceInfo, error) {
	if p.blockInfo == nil {
		return state.BlockDeviceInfo{}, p.err
	}
	return *p.blockInfo, p.err
}

type fakePoolManager struct {
	poolmanager.PoolManager
}

func (pm *fakePoolManager) Get(name string) (*storage.Config, error) {
	return nil, errors.NotFoundf("pool")
}

type fakeFilesystem struct {
	state.Filesystem
	tag    names.FilesystemTag
	params *state.FilesystemParams
	info   *state.FilesystemInfo
	status *corestatus.StatusInfo
}

func (v *fakeFilesystem) FilesystemTag() names.FilesystemTag {
	return v.tag
}

func (v *fakeFilesystem) Tag() names.Tag {
	return v.tag
}

func (v *fakeFilesystem) Params() (state.FilesystemParams, bool) {
	if v.params == nil {
		return state.FilesystemParams{}, false
	}
	return *v.params, true
}

func (v *fakeFilesystem) Info() (state.FilesystemInfo, error) {
	if v.info == nil {
		return state.FilesystemInfo{}, errors.NotProvisionedf("filesystem %v", v.tag.Id())
	}
	return *v.info, nil
}

func (v *fakeFilesystem) Status() (corestatus.StatusInfo, error) {
	if v.status == nil {
		return corestatus.StatusInfo{Status: corestatus.Attached}, nil
	}
	return *v.status, nil
}

type fakeFilesystemAttachment struct {
	state.FilesystemAttachment
	info *state.FilesystemAttachmentInfo
}

func (v *fakeFilesystemAttachment) Info() (state.FilesystemAttachmentInfo, error) {
	if v.info == nil {
		return state.FilesystemAttachmentInfo{}, errors.NotProvisionedf("filesystem attachment")
	}
	return *v.info, nil
}
