// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// ID represents a resource unique identifier.
type ID string

// NewID is a convince function for generating a new resource uuid.
func NewID() (ID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return ID(""), err
	}
	return ID(id.String()), nil
}

// ParseID returns a new ID from the given string. If the string is not a valid
// uuid an error satisfying [errors.NotValid] will be returned.
func ParseID(value string) (ID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", fmt.Errorf("id %q %w", value, errors.NotValid)
	}
	return ID(value), nil
}

// String implements the stringer interface for ID.
func (u ID) String() string {
	return string(u)
}

// Validate ensures the consistency of the ID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u ID) Validate() error {
	if u == "" {
		return fmt.Errorf("%wid cannot be empty", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return fmt.Errorf("id %q %w", u, errors.NotValid)
	}
	return nil
}
