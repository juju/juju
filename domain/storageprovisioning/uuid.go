// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// StorageAttachmentUUID represents the unique id for a storage attachment
// in the model.
type StorageAttachmentUUID uuid

// FilesystemAttachmentUUID represents the unique id for a storage filesystem
// attachment in the model.
type FilesystemAttachmentUUID uuid

// FilesystemUUID represents the unique id for a storage filesystem
// in the model.
type FilesystemUUID uuid

// VolumeAttachmentUUID represents the unique id for a storage volume
// attachment in the model.
type VolumeAttachmentUUID uuid

// VolumeUUID represents the unique id for a storage volume instance.
type VolumeUUID uuid

type uuid string

// NewStorageAttachmentUUID creates a new, valid storage attachment identifier.
func NewStorageAttachmentUUID() (StorageAttachmentUUID, error) {
	u, err := newUUID()
	return StorageAttachmentUUID(u), err
}

// NewFileystemAttachmentUUID creates a new, valid storage filesystem attachment
// identifier.
func NewFilesystemAttachmentUUID() (FilesystemAttachmentUUID, error) {
	u, err := newUUID()
	return FilesystemAttachmentUUID(u), err
}

// NewFilesystemUUID creates a new, valid storage filesystem identifier.
func NewFilesystemUUID() (FilesystemUUID, error) {
	u, err := newUUID()
	return FilesystemUUID(u), err
}

// NewVolumeAttachmentUUID creates a new, valid storage volume attachment
// identifier.
func NewVolumeAttachmentUUID() (VolumeAttachmentUUID, error) {
	u, err := newUUID()
	return VolumeAttachmentUUID(u), err
}

// NewVolumeUUID creates a new, valid storage volume identifier.
func NewVolumeUUID() (VolumeUUID, error) {
	u, err := newUUID()
	return VolumeUUID(u), err
}

// newUUID creates a new UUID using the internal uuid package.
func newUUID() (uuid, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return uuid(id.String()), nil
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u StorageAttachmentUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u FilesystemAttachmentUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u FilesystemUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u VolumeAttachmentUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u VolumeUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u uuid) String() string {
	return string(u)
}

// Validate returns an error if the [StorageAttachmentUUID] is not valid.
func (u StorageAttachmentUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [FilesystemAttachmentUUID] is not valid.
func (u FilesystemAttachmentUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [FilesystemUUID] is not valid.
func (u FilesystemUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [VolumeAttachmentUUID] is not valid.
func (u VolumeAttachmentUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [VolumeUUID] is not valid.
func (u VolumeUUID) Validate() error {
	return uuid(u).validate()
}

// validate checks that [uuid] is a valid uuid returning an error if it is not.
func (u uuid) validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	internaluuid.IsValidUUIDString(u.String())
	if !internaluuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}
