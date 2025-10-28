// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// FilesystemNotFound describes an error that occurs when no filesystem was
	// found in the model.
	FilesystemNotFound = errors.ConstError("filesystem not found")

	// FilesystemNotDead describes an error that occurs when the filesystem was
	// expected to be dead, but is not dead.
	FilesystemNotDead = errors.ConstError("filesystem not dead")

	// FilesystemAttachmentNotFound describes an error that occurs when no
	// filesystem attachment was found in the model.
	FilesystemAttachmentNotFound = errors.ConstError("filesystem attachment not found")

	// VolumeNotFound describes an error that occurs when no volume was
	// found in the model.
	VolumeNotFound = errors.ConstError("volume not found")

	// VolumeNotDead describes an error that occurs when the volume was expected
	// to be dead, but is not dead.
	VolumeNotDead = errors.ConstError("volume not dead")

	// VolumeAttachmentNotFound describes an error that occurs when no
	// volume attachment was found in the model.
	VolumeAttachmentNotFound = errors.ConstError("volume attachment not found")

	// VolumeAttachmentPlanNotFound is used when a volume attachment plan cannot
	// be found.
	VolumeAttachmentPlanNotFound = errors.ConstError("volume attachment plan not found")

	// VolumeAttachmentWithoutBlockDevice is used when a volume attachment does
	// not have an associated block device yet.
	VolumeAttachmentWithoutBlockDevice = errors.ConstError("volume attachment without block device")

	// VolumeAttachmentPlanAlreadyExists is used when a volume attachment plan
	// already exists for the given volume attachment.
	VolumeAttachmentPlanAlreadyExists = errors.ConstError("volume attachment plan already exists")

	// OwnershipScopeIncalculable is used when the ownership scope for a storage
	// instance cannot be calculated.
	OwnershipScopeIncalculable = errors.ConstError("ownership scope is incalculable")
)
