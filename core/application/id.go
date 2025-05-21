// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ID represents a application unique identifier.
type ID string

// NewID is a convenience function for generating a new application uuid.
func NewID() (ID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return ID(""), err
	}
	return ID(uuid.String()), nil
}

// ParseID returns a new ID from the given string. If the string is not a valid
// uuid an error satisfying [errors.NotValid] will be returned.
func ParseID(value string) (ID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", errors.Errorf("id %q %w", value, coreerrors.NotValid)
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
		return errors.Errorf("id cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("id %q %w", u, coreerrors.NotValid)
	}
	return nil
}
