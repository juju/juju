// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
)

// StorageInstanceDetails describes information about a storage instance.
type StorageInstanceDetails struct {
	UUID       string
	ID         string
	Owner      *unit.Name
	Kind       storage.StorageKind
	Life       life.Life
	Persistent bool
}

// VolumeDetails describes information about a volume with its attachments.
type VolumeDetails struct {
	StorageID   string
	Status      status.StatusInfo[status.StorageVolumeStatusType]
	Attachments []storage.VolumeAttachmentDetails
}

// FilesystemDetails describes information about a filesystem with its attachments.
type FilesystemDetails struct {
	StorageID   string
	Status      status.StatusInfo[status.StorageFilesystemStatusType]
	Attachments []storage.FilesystemAttachmentDetails
}
