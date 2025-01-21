// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// UUID represents a relation unique identifier.
type UUID string

// UnitUUID represents a relation unit unique identifier.
type UnitUUID string

// NewUUID is a convince function for generating a new relation uuid.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(id.String()), nil
}

// ParseUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseUUID(value string) (UUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", fmt.Errorf("id %q %w", value, errors.NotValid)
	}
	return UUID(value), nil
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return fmt.Errorf("%wuuid cannot be empty", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return fmt.Errorf("uuid %q %w", u, errors.NotValid)
	}
	return nil
}

// NewUnitUUID is a convince function for generating a new relation unit uuid.
func NewUnitUUID() (UnitUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return UnitUUID(""), err
	}
	return UnitUUID(id.String()), nil
}

// UnitUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseUnitUUID(value string) (UnitUUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", fmt.Errorf("id %q %w", value, errors.NotValid)
	}
	return UnitUUID(value), nil
}

// String implements the stringer interface for UUID.
func (u UnitUUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UnitUUID) Validate() error {
	if u == "" {
		return fmt.Errorf("%wuuid cannot be empty", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return fmt.Errorf("uuid %q %w", u, errors.NotValid)
	}
	return nil
}
