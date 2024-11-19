// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// UUID represents a charm unique identifier.
type UUID string

// NewUUID is a convince function for generating a new charm uuid.
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
}

// ParseUUID returns a new UUID from the given string. If the string is not a valid
// uuid an error satisfying [errors.NotValid] will be returned.
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
