// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// Id is a unqiue identifier for a machine.
type Id string

// NewId makes and returns a new machine [Id].
func NewId() (Id, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return "", fmt.Errorf("generating new machine id: %w", err)
	}

	return Id(uuid.String()), nil
}

// Validate returns an error if the [Id] is invalid. The error returned
// satisfies [errors.NotValid].
func (i Id) Validate() error {
	if i == "" {
		return fmt.Errorf("empty machine id%w", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(i)) {
		return fmt.Errorf("invalid machine id: %q%w", i, errors.Hide(errors.NotValid))
	}
	return nil
}

// String returns the [Id] as a string.
func (i Id) String() string {
	return string(i)
}
