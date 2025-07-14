// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// StorageFilesystemUUID represents the unique id for a storage filesystem
// instance.
type StorageFilesystemUUID uuid

// StorageVolumeUUID represents the unique id for a storage volume instance.
type StorageVolumeUUID uuid

type uuid string

// NewStorageFileystemUUID creates a new, valid storage filesystem identifier.
func NewStorageFileystemUUID() (StorageFilesystemUUID, error) {
	u, err := newUUID()
	return StorageFilesystemUUID(u), err
}

// NewStorageVolumeUUID creates a new, valid storage volume identifier.
func NewStorageVolumeUUID() (StorageVolumeUUID, error) {
	u, err := newUUID()
	return StorageVolumeUUID(u), err
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
func (u StorageFilesystemUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u StorageVolumeUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this uuid. This function
// satisfies the [fmt.Stringer] interface.
func (u uuid) String() string {
	return string(u)
}

// Validate returns an error if the [StorageFilesystemUUID] is not valid.
func (u StorageFilesystemUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [StorageVolumeUUID] is not valid.
func (u StorageVolumeUUID) Validate() error {
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
