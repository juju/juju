// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// TaskNotFound describes an error that occurs when the task being
	// operated on does not exist.
	TaskNotFound = errors.ConstError("task not found")
)
