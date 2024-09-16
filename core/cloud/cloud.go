// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// UUID represents a unique id within the Juju controller for a cloud.
type UUID string

// NewUUID generates a new cloud [UUID]
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), fmt.Errorf("creating new cloud id: %w", err)
	}
	return UUID(uuid.String()), nil
}

// String implements the stringer interface returning a string representation of
// the cloud UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the uuid. If the [UUID] is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return fmt.Errorf("cloud uuid cannot be empty%w", errors.Hide(errors.NotValid))
	}

	if !uuid.IsValidUUIDString(string(u)) {
		return fmt.Errorf("cloud uuid %q %w", u, errors.NotValid)
	}
	return nil
}
