// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// OperationNotFound describes an error that occurs when the given operation does not exist.
	OperationNotFound = errors.ConstError("operation not found")

	// TaskNotFound describes an error that occurs when the task being
	// operated on does not exist.
	TaskNotFound = errors.ConstError("task not found")

	// TaskNotPending describes an error that occurs when a pending task
	// is queried and does not have a pending status.
	TaskNotPending = errors.ConstError("task not pending")
)
