// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// ID is a unique identifier for a machine.
type ID string

// NewId makes and returns a new machine [ID].
func NewId() (ID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return "", fmt.Errorf("generating new machine id: %w", err)
	}

	return ID(uuid.String()), nil
}

// Validate returns an error if the [ID] is invalid. The error returned
// satisfies [errors.NotValid].
func (i ID) Validate() error {
	if i == "" {
		return fmt.Errorf("empty machine id%w", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(i)) {
		return fmt.Errorf("invalid machine id: %q%w", i, errors.Hide(errors.NotValid))
	}
	return nil
}

// String returns the [ID] as a string.
func (i ID) String() string {
	return string(i)
}
