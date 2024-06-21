// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// ID represents a charm unique identifier.
type ID string

// NewID is a convince function for generating a new charm uuid.
func NewID() (ID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return ID(""), err
	}
	return ID(uuid.String()), nil
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
