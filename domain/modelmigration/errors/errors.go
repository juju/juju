// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// InvalidUser is returned when the user is not valid to perform the
	// migration.
	InvalidUser = errors.ConstError("invalid user")
)
