// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facades/client/storage"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/state"
)

type mockBlockDeviceGetter struct {
	blockDevices func(string) ([]blockdevice.BlockDevice, error)
}

func (b *mockBlockDeviceGetter) BlockDevices(_ context.Context, machineId string) ([]blockdevice.BlockDevice, error) {
	if b.blockDevices != nil {
		return b.blockDevices(machineId)
	}
	return []blockdevice.BlockDevice{}, nil
}

type mockStorageAccessor struct {
	storageInstance                     func(names.StorageTag) (state.StorageInstance, error)
	allStorageInstances                 func() ([]state.StorageInstance, error)
	storageInstanceAttachments          func(names.StorageTag) ([]state.StorageAttachment, error)
	storageInstanceVolume               func(names.StorageTag) (state.Volume, error)
	volumeAttachment                    func(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	storageInstanceFilesystem           func(names.StorageTag) (state.Filesystem, error)
	storageInstanceFilesystemAttachment func(m names.Tag, f names.FilesystemTag) (state.FilesystemAttachment, error)
	volume                              func(tag names.VolumeTag) (state.Volume, error)
	machineVolumeAttachments            func(machine names.MachineTag) ([]state.VolumeAttachment, error)
	volumeAttachments                   func(volume names.VolumeTag) ([]state.VolumeAttachment, error)
	volumeAttachmentPlan                func(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
	volumeAttachmentPlans               func(names.VolumeTag) ([]state.VolumeAttachmentPlan, error)
	allVolumes                          func() ([]state.Volume, error)
	filesystem                          func(tag names.FilesystemTag) (state.Filesystem, error)
	machineFilesystemAttachments        func(machine names.MachineTag) ([]state.FilesystemAttachment, error)
	filesystemAttachments               func(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error)
	allFilesystems                      func() ([]state.Filesystem, error)
	addStorageForUnit                   func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error)
	destroyStorageInstance              func(names.StorageTag, bool, bool) error
	releaseStorageInstance              func(names.StorageTag, bool, bool) error
	attachStorage                       func(names.StorageTag, names.UnitTag) error
	detachStorage                       func(names.StorageTag, names.UnitTag, bool) error
	addExistingFilesystem               func(state.FilesystemInfo, *state.VolumeInfo, string) (names.StorageTag, error)
}

func (st *mockStorageAccessor) VolumeAccess() storage.StorageVolume {
	return st
}

func (st *mockStorageAccessor) FilesystemAccess() storage.StorageFile {
	return st
}

func (st *mockStorageAccessor) StorageInstance(s names.StorageTag) (state.StorageInstance, error) {
	return st.storageInstance(s)
}

func (st *mockStorageAccessor) AllStorageInstances() ([]state.StorageInstance, error) {
	return st.allStorageInstances()
}

func (st *mockStorageAccessor) StorageAttachments(tag names.StorageTag) ([]state.StorageAttachment, error) {
	return st.storageInstanceAttachments(tag)
}

func (st *mockStorageAccessor) FilesystemAttachment(m names.Tag, f names.FilesystemTag) (state.FilesystemAttachment, error) {
	return st.storageInstanceFilesystemAttachment(m, f)
}

func (st *mockStorageAccessor) StorageInstanceFilesystem(s names.StorageTag) (state.Filesystem, error) {
	return st.storageInstanceFilesystem(s)
}

func (st *mockStorageAccessor) StorageInstanceVolume(s names.StorageTag) (state.Volume, error) {
	return st.storageInstanceVolume(s)
}

func (st *mockStorageAccessor) VolumeAttachment(m names.Tag, v names.VolumeTag) (state.VolumeAttachment, error) {
	return st.volumeAttachment(m, v)
}

func (st *mockStorageAccessor) VolumeAttachmentPlan(host names.Tag, volume names.VolumeTag) (state.VolumeAttachmentPlan, error) {
	return st.volumeAttachmentPlan(host, volume)
}

func (st *mockStorageAccessor) VolumeAttachmentPlans(volume names.VolumeTag) ([]state.VolumeAttachmentPlan, error) {
	// st.MethodCall(st, "VolumeAttachmentPlans", volume)
	return st.volumeAttachmentPlans(volume)
}

func (st *mockStorageAccessor) AllVolumes() ([]state.Volume, error) {
	return st.allVolumes()
}

func (st *mockStorageAccessor) VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
	return st.volumeAttachments(volume)
}

func (st *mockStorageAccessor) MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error) {
	return st.machineVolumeAttachments(machine)
}

func (st *mockStorageAccessor) Volume(tag names.VolumeTag) (state.Volume, error) {
	return st.volume(tag)
}

func (st *mockStorageAccessor) AllFilesystems() ([]state.Filesystem, error) {
	return st.allFilesystems()
}

func (st *mockStorageAccessor) FilesystemAttachments(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error) {
	return st.filesystemAttachments(filesystem)
}

func (st *mockStorageAccessor) MachineFilesystemAttachments(machine names.MachineTag) ([]state.FilesystemAttachment, error) {
	return st.machineFilesystemAttachments(machine)
}

func (st *mockStorageAccessor) Filesystem(tag names.FilesystemTag) (state.Filesystem, error) {
	return st.filesystem(tag)
}

func (st *mockStorageAccessor) AddStorageForUnit(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
	return st.addStorageForUnit(u, name, cons)
}

func (st *mockStorageAccessor) AttachStorage(storage names.StorageTag, unit names.UnitTag) error {
	return st.attachStorage(storage, unit)
}

func (st *mockStorageAccessor) DetachStorage(storage names.StorageTag, unit names.UnitTag, force bool, maxWait time.Duration) error {
	return st.detachStorage(storage, unit, force)
}

func (st *mockStorageAccessor) DestroyStorageInstance(tag names.StorageTag, destroyAttached bool, force bool, maxWait time.Duration) error {
	return st.destroyStorageInstance(tag, destroyAttached, force)
}

func (st *mockStorageAccessor) ReleaseStorageInstance(tag names.StorageTag, destroyAttached bool, force bool, maxWait time.Duration) error {
	return st.releaseStorageInstance(tag, destroyAttached, force)
}

func (st *mockStorageAccessor) UnitStorageAttachments(tag names.UnitTag) ([]state.StorageAttachment, error) {
	panic("should not be called")
}

func (st *mockStorageAccessor) AddExistingFilesystem(f state.FilesystemInfo, v *state.VolumeInfo, s string) (names.StorageTag, error) {
	return st.addExistingFilesystem(f, v, s)
}

type mockVolume struct {
	state.Volume
	tag     names.VolumeTag
	storage *names.StorageTag
	info    *state.VolumeInfo
	life    state.Life
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

func (m *mockVolume) Life() state.Life {
	return m.life
}

func (m *mockVolume) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Attached}, nil
}

type mockFilesystem struct {
	state.Filesystem
	tag     names.FilesystemTag
	storage *names.StorageTag
	volume  *names.VolumeTag
	info    *state.FilesystemInfo
	life    state.Life
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

func (m *mockFilesystem) Life() state.Life {
	return m.life
}

func (m *mockFilesystem) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Attached}, nil
}

type mockFilesystemAttachment struct {
	state.FilesystemAttachment
	filesystem names.FilesystemTag
	machine    names.MachineTag
	info       *state.FilesystemAttachmentInfo
	life       state.Life
}

func (m *mockFilesystemAttachment) Filesystem() names.FilesystemTag {
	return m.filesystem
}

func (m *mockFilesystemAttachment) Host() names.Tag {
	return m.machine
}

func (m *mockFilesystemAttachment) Info() (state.FilesystemAttachmentInfo, error) {
	if m.info != nil {
		return *m.info, nil
	}
	return state.FilesystemAttachmentInfo{}, errors.NotProvisionedf("filesystem attachment")
}

func (m *mockFilesystemAttachment) Life() state.Life {
	return m.life
}

type mockStorageInstance struct {
	state.StorageInstance
	kind       state.StorageKind
	owner      names.Tag
	storageTag names.Tag
	life       state.Life
}

func (m *mockStorageInstance) Kind() state.StorageKind {
	return m.kind
}

func (m *mockStorageInstance) Owner() (names.Tag, bool) {
	return m.owner, m.owner != nil
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

func (m *mockStorageInstance) Life() state.Life {
	return m.life
}

type mockStorageAttachment struct {
	state.StorageAttachment
	storage *mockStorageInstance
	life    state.Life
}

func (m *mockStorageAttachment) StorageInstance() names.StorageTag {
	return m.storage.Tag().(names.StorageTag)
}

func (m *mockStorageAttachment) Unit() names.UnitTag {
	return m.storage.owner.(names.UnitTag)
}

func (m *mockStorageAttachment) Life() state.Life {
	return m.life
}

type mockVolumeAttachmentPlan struct {
	VolumeTag names.VolumeTag
	HostTag   names.MachineTag
	info      *state.VolumeAttachmentPlanInfo
	life      state.Life
	blk       *state.BlockDeviceInfo
}

func (v *mockVolumeAttachmentPlan) Volume() names.VolumeTag {
	return v.VolumeTag
}

func (v *mockVolumeAttachmentPlan) Machine() names.MachineTag {
	return v.HostTag
}

func (v *mockVolumeAttachmentPlan) PlanInfo() (state.VolumeAttachmentPlanInfo, error) {
	if v.info != nil {
		return *v.info, nil
	}
	return state.VolumeAttachmentPlanInfo{}, nil
}

func (v *mockVolumeAttachmentPlan) BlockDeviceInfo() (state.BlockDeviceInfo, error) {
	if v.blk != nil {
		return *v.blk, nil
	}
	return state.BlockDeviceInfo{}, nil
}

func (v *mockVolumeAttachmentPlan) Life() state.Life {
	return v.life
}

type mockVolumeAttachment struct {
	VolumeTag names.VolumeTag
	HostTag   names.Tag
	info      *state.VolumeAttachmentInfo
	life      state.Life
}

func (va *mockVolumeAttachment) Volume() names.VolumeTag {
	return va.VolumeTag
}

func (va *mockVolumeAttachment) Host() names.Tag {
	return va.HostTag
}

func (va *mockVolumeAttachment) Life() state.Life {
	return va.life
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

type mockUnit struct {
	assignedMachine string
}

func (u *mockUnit) AssignedMachineId() (string, error) {
	return u.assignedMachine, nil
}

type mockState struct {
	unitName        string
	unitErr         string
	assignedMachine string
}

func (st *mockState) Unit(unitName string) (storage.Unit, error) {
	if st.unitErr != "" {
		return nil, errors.New(st.unitErr)
	}
	if unitName == st.unitName {
		return &mockUnit{assignedMachine: st.assignedMachine}, nil
	}
	return nil, errors.NotFoundf(unitName)
}
