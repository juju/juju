// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/errors"
)

// Name is a unique name identifier for a machine.
type Name string

// Validate returns an error if the [Name] is invalid. The error returned
// satisfies [errors.NotValid].
func (n Name) Validate() error {
	if n == "" {
		return fmt.Errorf("empty machine name%w", errors.Hide(errors.NotValid))
	}
	return nil
}

// String returns the [Name] as a string.
func (i Name) String() string {
	return string(i)
}
