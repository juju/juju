// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// ID represents a unique id within the Juju controller for a cloud.
type ID string

// NewID generates a new credential [ID]
func NewID() (ID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return ID(""), fmt.Errorf("creating new credential id: %w", err)
	}
	return ID(uuid.String()), nil
}

// String implements the stringer interface returning a string representation of
// the credential ID.
func (i ID) String() string {
	return string(i)
}

// Validate ensures the consistency of the id. If the [ID] is invalid an error
// satisfying [errors.NotValid] will be returned.
func (i ID) Validate() error {
	if i == "" {
		return fmt.Errorf("credential id cannot be empty%w", errors.Hide(errors.NotValid))
	}

	if !uuid.IsValidUUIDString(string(i)) {
		return fmt.Errorf("credential id %q %w", i, errors.NotValid)
	}
	return nil
}
