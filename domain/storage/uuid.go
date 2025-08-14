// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// StorageInstanceUUID uniquely identifies a storage instance in the model.
type StorageInstanceUUID uuid

// StoragePoolUUID uniquely identifies a storage pool in the model.
type StoragePoolUUID uuid

type uuid string

// NewStorageInstanceUUID creates a new, valid storage instance identifier.
func NewStorageInstanceUUID() (StorageInstanceUUID, error) {
	u, err := newUUID()
	return StorageInstanceUUID(u), err
}

// GenStorageInstanceUUID generates a new [StorageInstanceUUID] for testing
// purposes.
func GenStorageInstanceUUID(c interface{ Fatal(...any) }) StorageInstanceUUID {
	uuid, err := NewStorageInstanceUUID()
	if err != nil {
		c.Fatal(err)
	}
	return uuid
}

// NewStoragePoolUUID creates a new, valid storage pool identifier.
func NewStoragePoolUUID() (StoragePoolUUID, error) {
	u, err := newUUID()
	return StoragePoolUUID(u), err
}

// GenStoragePoolUUID generates a new [StoragePoolUUID] for testing
// purposes.
func GenStoragePoolUUID(c interface{ Fatal(...any) }) StoragePoolUUID {
	uuid, err := NewStoragePoolUUID()
	if err != nil {
		c.Fatal(err)
	}
	return uuid
}

// newUUID creates a new UUID using the internal uui package.
func newUUID() (uuid, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return uuid(id.String()), nil
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u StorageInstanceUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u StoragePoolUUID) String() string {
	return uuid(u).String()
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u uuid) String() string {
	return string(u)
}

// Validate returns an error if the [StorageInstanceUUID] is not valid.
func (u StorageInstanceUUID) Validate() error {
	return uuid(u).validate()
}

// Validate returns an error if the [StoragePoolUUID] is not valid.
func (u StoragePoolUUID) Validate() error {
	return uuid(u).validate()
}

// validate checks that [uuid] is a valid uuid returning an error if it is not.
func (u uuid) validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	internaluuid.IsValidUUIDString(u.String())
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}
