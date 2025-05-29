// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/internal/uuid"
)

// UUID represents a storage unique identifier.
type UUID string

// NewUUID is a convenience function for generating a new storage uuid.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return UUID(id.String()), nil
}

// FilesystemUUID represents a filesystem unique identifier.
type FilesystemUUID string

// NewFilesystemUUID is a convenience function for generating a new filesystem uuid.
func NewFilesystemUUID() (FilesystemUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return FilesystemUUID(id.String()), nil
}

// String implements the stringer interface.
func (u FilesystemUUID) String() string {
	return string(u)
}

// VolumeUUID represents a volume unique identifier.
type VolumeUUID string

// NewVolumeUUID is a convenience function for generating a new volum uuid.
func NewVolumeUUID() (VolumeUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return VolumeUUID(id.String()), nil
}

// String implements the stringer interface.
func (u VolumeUUID) String() string {
	return string(u)
}

// FilesystemAttachmentUUID represents a filesystem attachment unique identifier.
type FilesystemAttachmentUUID string

// NewFilesystemAttachmentUUID is a convenience function for generating a new filesystem attachment uuid.
func NewFilesystemAttachmentUUID() (FilesystemAttachmentUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return FilesystemAttachmentUUID(id.String()), nil
}

// String implements the stringer interface.
func (u FilesystemAttachmentUUID) String() string {
	return string(u)
}

// VolumeAttachmentUUID represents a volume attachment unique identifier.
type VolumeAttachmentUUID string

// NewVolumeAttachmentUUID is a convenience function for generating a new volume attachment uuid.
func NewVolumeAttachmentUUID() (VolumeAttachmentUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return VolumeAttachmentUUID(id.String()), nil
}

// String implements the stringer interface.
func (u VolumeAttachmentUUID) String() string {
	return string(u)
}
