// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type uuid string

func newUUID() (uuid, error) {
	id, err := internaluuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return uuid(id.String()), nil
}

func (u uuid) validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}

// StoragePoolUUID uniquely identifies a storage pool.
type StoragePoolUUID uuid

// NewStoragePoolUUID creates a new, valid storage pool identifier.
func NewStoragePoolUUID() (StoragePoolUUID, error) {
	u, err := newUUID()
	return StoragePoolUUID(u), err
}

// Validate returns an error if the receiver is not a valid UUID.
func (u StoragePoolUUID) Validate() error {
	return uuid(u).validate()
}

// String returns the identifier in string form.
func (u StoragePoolUUID) String() string {
	return string(u)
}
