// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// CAASModel contains functionality that is specific to an
// Containers-As-A-Service (CAAS) model. It embeds a Model so that
// all generic Model functionality is also available.
type CAASModel struct {
	// TODO(caas) - this is all still messy until things shake out.
	*Model
	// TODO(caas) - placeholder until storage done.
	noopStorage

	mb modelBackend
}

// CAASModel returns an Containers-As-A-Service (CAAS) model.
func (m *Model) CAASModel() (*CAASModel, error) {
	if m.Type() != ModelTypeCAAS {
		return nil, errors.NotSupportedf("called CAASModel() on a non-CAAS Model")
	}
	return &CAASModel{
		Model: m,
		mb:    m.st,
	}, nil
}

// noopStorage implements the storageAccessor interface and returns
// NotSupported errors for all methods.
// Used as a placeholder until storage is implemented.
type noopStorage struct{}

func (noopStorage) StorageInstance(names.StorageTag) (StorageInstance, error) {
	return nil, errors.NotSupportedf("StorageInstance")
}

func (noopStorage) AllStorageInstances() ([]StorageInstance, error) {
	return nil, errors.NotSupportedf("AllStorageInstances")
}

func (noopStorage) StorageAttachments(names.StorageTag) ([]StorageAttachment, error) {
	return nil, errors.NotSupportedf("StorageAttachments")
}

func (noopStorage) FilesystemAttachment(names.MachineTag, names.FilesystemTag) (FilesystemAttachment, error) {
	return nil, errors.NotSupportedf("FilesystemAttachment")
}

func (noopStorage) StorageInstanceFilesystem(names.StorageTag) (Filesystem, error) {
	return nil, errors.NotSupportedf("StorageInstanceFilesystem")
}

func (noopStorage) StorageInstanceVolume(names.StorageTag) (Volume, error) {
	return nil, errors.NotSupportedf("StorageInstance")
}

func (noopStorage) VolumeAttachment(names.MachineTag, names.VolumeTag) (VolumeAttachment, error) {
	return nil, errors.NotSupportedf("StorageInstanceVolume")
}

func (noopStorage) WatchStorageAttachment(names.StorageTag, names.UnitTag) NotifyWatcher {
	return nil
}

func (noopStorage) WatchStorageAttachments(names.UnitTag) StringsWatcher {
	return nil
}

func (noopStorage) WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) NotifyWatcher {
	return nil
}

func (noopStorage) WatchVolumeAttachment(names.MachineTag, names.VolumeTag) NotifyWatcher {
	return nil
}

func (noopStorage) WatchBlockDevices(names.MachineTag) NotifyWatcher {
	return nil
}

func (noopStorage) BlockDevices(names.MachineTag) ([]BlockDeviceInfo, error) {
	return nil, errors.NotSupportedf("BlockDevices")
}

func (noopStorage) AllVolumes() ([]Volume, error) {
	return nil, errors.NotSupportedf("AllVolumes")
}

func (noopStorage) VolumeAttachments(volume names.VolumeTag) ([]VolumeAttachment, error) {
	return nil, errors.NotSupportedf("VolumeAttachments")
}

func (noopStorage) MachineVolumeAttachments(machine names.MachineTag) ([]VolumeAttachment, error) {
	return nil, errors.NotSupportedf("MachineVolumeAttachments")
}

func (noopStorage) Volume(tag names.VolumeTag) (Volume, error) {
	return nil, errors.NotSupportedf("Volume")
}

func (noopStorage) AllFilesystems() ([]Filesystem, error) {
	return nil, errors.NotSupportedf("AllFilesystems")
}

func (noopStorage) FilesystemAttachments(filesystem names.FilesystemTag) ([]FilesystemAttachment, error) {
	return nil, errors.NotSupportedf("FilesystemAttachments")
}

func (noopStorage) MachineFilesystemAttachments(machine names.MachineTag) ([]FilesystemAttachment, error) {
	return nil, errors.NotSupportedf("MachineFilesystemAttachments")
}

func (noopStorage) Filesystem(tag names.FilesystemTag) (Filesystem, error) {
	return nil, errors.NotSupportedf("Filesystem")
}

func (noopStorage) AddStorageForUnit(tag names.UnitTag, name string, cons StorageConstraints) ([]names.StorageTag, error) {
	return nil, errors.NotSupportedf("AddStorageForUnit")
}

func (noopStorage) AttachStorage(names.StorageTag, names.UnitTag) error {
	return errors.NotSupportedf("AttachStorage")
}

func (noopStorage) DetachStorage(names.StorageTag, names.UnitTag) error {
	return errors.NotSupportedf("DetachStorage")
}

func (noopStorage) DestroyStorageInstance(names.StorageTag, bool) error {
	return errors.NotSupportedf("DestroyStorageInstance")
}

func (noopStorage) ReleaseStorageInstance(names.StorageTag, bool) error {
	return errors.NotSupportedf("ReleaseStorageInstance")
}

func (noopStorage) UnitStorageAttachments(names.UnitTag) ([]StorageAttachment, error) {
	return nil, errors.NotSupportedf("UnitStorageAttachments")
}

func (noopStorage) AddExistingFilesystem(f FilesystemInfo, v *VolumeInfo, storageName string) (names.StorageTag, error) {
	return names.StorageTag{}, errors.NotSupportedf("AddExistingFilesystem")
}

func (noopStorage) StorageAttachment(names.StorageTag, names.UnitTag) (StorageAttachment, error) {
	return nil, errors.NotSupportedf("StorageAttachment")
}

func (noopStorage) DestroyUnitStorageAttachments(unit names.UnitTag) (err error) {
	return errors.NotSupportedf("DestroyUnitStorageAttachments")
}

func (noopStorage) RemoveStorageAttachment(s names.StorageTag, u names.UnitTag) error {
	return errors.NotSupportedf("RemoveStorageAttachment")
}
