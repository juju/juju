// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// StorageInstanceUUID uniquely identifies a storage instance in the model.
type StorageInstanceUUID baseUUID

// StoragePoolUUID uniquely identifies a storage pool in the model.
type StoragePoolUUID baseUUID

// baseUUID is a type that is used to build strongly typed entity uuids within
// this domain.
type baseUUID string

// NewStorageInstanceUUID creates a new, valid storage instance identifier.
func NewStorageInstanceUUID() (StorageInstanceUUID, error) {
	u, err := newUUID()
	return StorageInstanceUUID(u), err
}

// NewStoragePoolUUID creates a new, valid storage pool identifier.
func NewStoragePoolUUID() (StoragePoolUUID, error) {
	u, err := newUUID()
	return StoragePoolUUID(u), err
}

// newUUID creates a new UUID using the internal uui package.
func newUUID() (baseUUID, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return baseUUID(id.String()), nil
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u StorageInstanceUUID) String() string {
	return baseUUID(u).String()
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u StoragePoolUUID) String() string {
	return baseUUID(u).String()
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u baseUUID) String() string {
	return string(u)
}

// Validate returns an error if the [StorageInstanceUUID] is not valid.
func (u StorageInstanceUUID) Validate() error {
	return baseUUID(u).validate()
}

// Validate returns an error if the [StoragePoolUUID] is not valid.
func (u StoragePoolUUID) Validate() error {
	return baseUUID(u).validate()
}

// validate checks that [uuid] is a valid uuid returning an error if it is not.
func (u baseUUID) validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	internaluuid.IsValidUUIDString(u.String())
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}
