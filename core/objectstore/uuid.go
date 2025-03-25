// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// UUID represents a object store unique identifier.
type UUID string

// NewUUID is a convince function for generating a new object store uuid.
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
}

// ParseUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseUUID(value string) (UUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", errors.Errorf("id %q: %w", value, coreerrors.NotValid)
	}
	return UUID(value), nil
}

// IsEmpty returns true if the UUID is empty.
func (u UUID) IsEmpty() bool {
	return u == ""
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return errors.Errorf("uuid cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("uuid %q %w", u, coreerrors.NotValid)
	}
	return nil
}
