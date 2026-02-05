// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"

	"github.com/juju/juju/internal/errors"
)

// StoragePoolAttributeInvalid represents an error that occurs when parsing the
// attributes of a storage pool and one of the key's contains a value that is
// invalid for use.
//
// StoragePoolAttributeInvalid implements the [error] interface.
type StoragePoolAttributeInvalid struct {
	// Key is the attribute key where the invalid value was found.
	Key string

	// Message is a short error description explaining why the key value is not
	// fit for use.
	Message string
}

const (
	// FilesystemNotFound describes an error that occurs when the filesystem being operated
	// on does not exist.
	FilesystemNotFound = errors.ConstError("filesystem not found")

	// InvalidStorageName represents an invalid storage name.
	InvalidStorageName = errors.ConstError("invalid storage name")

	// ProviderTypeInvalid is used when a storage provider type value is not
	// valid for use within the model.
	ProviderTypeInvalid = errors.ConstError("provider type is invalid")

	// ProviderTypeNotFound is used when a storage provider type is not found.
	ProviderTypeNotFound = errors.ConstError("storage provider type not found")

	// StorageAttachmentNotFound is used when a storage attachment cannot be found.
	StorageAttachmentNotFound = errors.ConstError("storage attachment not found")

	// StorageInstanceNotFound describes an error that occurs when the storage
	// instance being operated on does not exist.
	StorageInstanceNotFound = errors.ConstError("storage instance not found")

	// StoragePoolAlreadyExists is used when a storage pool already exists.
	StoragePoolAlreadyExists = errors.ConstError("storage pool already exists")

	// StoragePoolNameInvalid is used when a storage pool name is invalid.
	StoragePoolNameInvalid = errors.ConstError("storage pool name is invalid")

	// StoragePoolNotFound is used when a storage pool is not found.
	StoragePoolNotFound = errors.ConstError("storage pool is not found")

	// VolumeNotFound describes an error that occurs when the volume being operated
	// on does not exist.
	VolumeNotFound = errors.ConstError("volume not found")
)

// Error returns a formatted string error message describing the the storage
// pool attribute key that is invalid and why it is considered invalid.
func (e StoragePoolAttributeInvalid) Error() string {
	return fmt.Sprintf("invalid value for attribute %q: %s", e.Key, e.Message)
}
