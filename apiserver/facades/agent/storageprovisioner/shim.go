// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
)

type StorageBackend interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	AllStorageInstances() ([]state.StorageInstance, error)
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	ReleaseStorageInstance(names.StorageTag, bool, bool, time.Duration) error
	DetachStorage(names.StorageTag, names.UnitTag, bool, time.Duration) error

	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	FilesystemAttachment(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)

	Volume(names.VolumeTag) (state.Volume, error)
	VolumeAttachment(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)
	VolumeAttachmentPlan(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
	VolumeAttachmentPlans(volume names.VolumeTag) ([]state.VolumeAttachmentPlan, error)

	RemoveFilesystem(names.FilesystemTag) error
	RemoveFilesystemAttachment(names.Tag, names.FilesystemTag, bool) error
	RemoveVolume(names.VolumeTag) error
	RemoveVolumeAttachment(names.Tag, names.VolumeTag, bool) error
	DetachFilesystem(names.Tag, names.FilesystemTag) error
	DestroyFilesystem(names.FilesystemTag, bool) error
	DetachVolume(names.Tag, names.VolumeTag, bool) error
	DestroyVolume(names.VolumeTag, bool) error

	SetFilesystemInfo(names.FilesystemTag, state.FilesystemInfo) error
	SetFilesystemAttachmentInfo(names.Tag, names.FilesystemTag, state.FilesystemAttachmentInfo) error
	SetVolumeInfo(names.VolumeTag, state.VolumeInfo) error
	SetVolumeAttachmentInfo(names.Tag, names.VolumeTag, state.VolumeAttachmentInfo) error

	CreateVolumeAttachmentPlan(names.Tag, names.VolumeTag, state.VolumeAttachmentPlanInfo) error
	RemoveVolumeAttachmentPlan(names.Tag, names.VolumeTag, bool) error
	SetVolumeAttachmentPlanBlockInfo(machineTag names.Tag, volumeTag names.VolumeTag, info state.BlockDeviceInfo) error
}

// NewStorageBackend creates a Backend from the given *state.State.
func NewStorageBackend(st *state.State) (StorageBackend, error) {
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	return sb, nil
}
