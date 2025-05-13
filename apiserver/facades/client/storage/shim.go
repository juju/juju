// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"time"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/state"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

type storageAccess interface {
	storageInterface
	storageVolume
	storageFile
}

type storageInterface interface {
	// StorageInstance is required for storage functionality.
	StorageInstance(names.StorageTag) (state.StorageInstance, error)

	// AllStorageInstances is required for storage functionality.
	AllStorageInstances() ([]state.StorageInstance, error)

	// StorageAttachments is required for storage functionality.
	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)

	// UnitStorageAttachments returns the storage attachments for the
	// identified unit.
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)

	// DestroyStorageInstance destroys the storage instance with the specified tag.
	DestroyStorageInstance(names.StorageTag, bool, bool, time.Duration) error

	// ReleaseStorageInstance releases the storage instance with the specified tag.
	ReleaseStorageInstance(names.StorageTag, bool, bool, time.Duration) error
}

type storageVolume interface {
	storagecommon.VolumeAccess

	// AllVolumes is required for volume functionality.
	AllVolumes() ([]state.Volume, error)

	// VolumeAttachments is required for volume functionality.
	VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error)

	VolumeAttachmentPlans(volume names.VolumeTag) ([]state.VolumeAttachmentPlan, error)

	// MachineVolumeAttachments is required for volume functionality.
	MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error)

	// Volume is required for volume functionality.
	Volume(tag names.VolumeTag) (state.Volume, error)
}

type storageFile interface {
	storagecommon.FilesystemAccess

	// AllFilesystems is required for filesystem functionality.
	AllFilesystems() ([]state.Filesystem, error)

	// FilesystemAttachments is required for filesystem functionality.
	FilesystemAttachments(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error)

	// MachineFilesystemAttachments is required for filesystem functionality.
	MachineFilesystemAttachments(machine names.MachineTag) ([]state.FilesystemAttachment, error)

	// Filesystem is required for filesystem functionality.
	Filesystem(tag names.FilesystemTag) (state.Filesystem, error)
}

var getStorageAccessor = func(
	st *state.State,
) (storageAccess, error) {
	sb, err := state.NewStorageConfigBackend(st)
	if err != nil {
		return nil, err
	}
	return sb, nil
}

type blockDeviceGetter interface {
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
}
