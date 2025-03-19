// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// NotFound describes an error that occurs when a controller cannot be
	// found.
	NotFound = errors.ConstError("controller not found")
)
