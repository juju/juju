// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import "github.com/juju/juju/internal/errors"

// Origin represents the origin of a secret backend as recorded in the
// secret_backend_origin lookup table.
type Origin int

const (
	// BuiltIn indicates a built-in secret backend managed by Juju itself.
	BuiltIn Origin = iota
	// User indicates a secret backend that was created by a user.
	User
)

// Value returns the string value corresponding to this origin as stored in the
// controller database lookup table `secret_backend_origin`.
func (o Origin) Value() (string, error) {
	switch o {
	case BuiltIn:
		return "built-in", nil
	case User:
		return "user", nil
	}
	return "", errors.Errorf("invalid origin value %d", o)
}
