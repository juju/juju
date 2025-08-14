// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ID represents a charm unique identifier.
type ID string

// NewID is a convenience function for generating a new charm uuid.
func NewID() (ID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return ID(""), err
	}
	return ID(uuid.String()), nil
}

// GenCharmID can be used in testing for generating a charm ID that is
// checked for subsequent errors.
func GenCharmID(c interface{ Fatal(...any) }) ID {
	id, err := NewID()
	if err != nil {
		c.Fatal(err)
	}
	return id
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
