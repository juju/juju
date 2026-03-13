// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/internal/errors"
)

// Origin represents the origin of a secret backend as recorded in the
// secret_backend_origin lookup table.
type Origin string

const (
	// BuiltIn indicates a built-in secret backend managed by Juju itself.
	BuiltIn Origin = "built-in"
	// User indicates a secret backend that was created by a user.
	User Origin = "user"
)

// String returns m as a string.
func (o Origin) String() string {
	return string(o)
}

// IsValid returns true if the value of Type is a known valid type.
func (o Origin) IsValid() bool {
	switch o {
	case BuiltIn, User:
		return true
	}
	return false
}

// ParseOrigin parses a string into an Origin.
func ParseOrigin(s string) (Origin, error) {
	switch s {
	case string(BuiltIn):
		return BuiltIn, nil
	case string(User):
		return User, nil
	}
	return "", errors.Errorf("unknown origin %q", s)
}
