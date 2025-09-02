// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ActionNotFound describes an error that occurs when the action being
	// operated on does not exist.
	ActionNotFound = errors.ConstError("action not found")
)
