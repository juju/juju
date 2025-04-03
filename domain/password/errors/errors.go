// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// UnitNotFound describes an error that occurs when the unit being operated
	// on does not exist.
	UnitNotFound = errors.ConstError("unit not found")

	// InvalidPassword describes an error that occurs when the password is not
	// valid.
	InvalidPassword = errors.ConstError("invalid password")

	// EmptyPassword describes an error that occurs when the password is empty.
	EmptyPassword = errors.ConstError("empty password")
)
