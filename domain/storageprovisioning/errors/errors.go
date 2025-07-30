// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// FilesystemNotFound describes an error that occurs when no filesystem was
	// found in the model.
	FilesystemNotFound = errors.ConstError("filesystem not found")

	// FilesystemAttachmentNotFound describes an error that occurs when no
	// filesystem attachment was found in the model.
	FilesystemAttachmentNotFound = errors.ConstError("filesystem attachment not found")

	// VolumeNotFound describes an error that occurs when no volume was
	// found in the model.
	VolumeNotFound = errors.ConstError("volume not found")

	// VolumeAttachmentNotFound describes an error that occurs when no
	// volume attachment was found in the model.
	VolumeAttachmentNotFound = errors.ConstError("volume attachment not found")

	// VolumeAttachmentPlanNotFound is used when a volume attachment plan cannot
	// be found.
	VolumeAttachmentPlanNotFound = errors.ConstError("volume attachment plan not found")

	// BlockDeviceNotFound is used when a block device is not found.
	BlockDeviceNotFound = errors.ConstError("block device not found")
)
