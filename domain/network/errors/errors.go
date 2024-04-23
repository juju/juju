// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/errors"

const (
	// ErrAlreadyExists is returned when a space already exists.
	ErrAlreadyExists = errors.ConstError("already exists")
)
