// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	jujuerrors "github.com/juju/errors"
)

const (
	// The below are the generic error types that we previously got from
	// juju/errors.

	// Timeout represents an error on timeout.
	Timeout = jujuerrors.Timeout

	// NotFound represents an error when something has not been found.
	NotFound = jujuerrors.NotFound

	// UserNotFound represents an error when a non-existent user is looked up.
	UserNotFound = jujuerrors.UserNotFound

	// Unauthorized represents an error when an operation is unauthorized.
	Unauthorized = jujuerrors.Unauthorized

	// NotImplemented represents an error when something is not implemented.
	NotImplemented = jujuerrors.NotImplemented

	// AlreadyExists represents and error when something already exists.
	AlreadyExists = jujuerrors.AlreadyExists

	// NotSupported represents an error when something is not supported.
	NotSupported = jujuerrors.NotSupported

	// NotValid represents an error when something is not valid.
	NotValid = jujuerrors.NotValid

	// NotProvisioned represents an error when something is not yet provisioned.
	NotProvisioned = jujuerrors.NotProvisioned

	// NotAssigned represents an error when something is not yet assigned to
	// something else.
	NotAssigned = jujuerrors.NotAssigned

	// BadRequest represents an error when a request has bad parameters.
	BadRequest = jujuerrors.BadRequest

	// MethodNotAllowed represents an error when an HTTP request
	// is made with an inappropriate method.
	MethodNotAllowed = jujuerrors.MethodNotAllowed

	// Forbidden represents an error when a request cannot be completed because of
	// missing privileges.
	Forbidden = jujuerrors.Forbidden

	// QuotaLimitExceeded is emitted when an action failed due to a quota limit check.
	QuotaLimitExceeded = jujuerrors.QuotaLimitExceeded

	// NotYetAvailable is the error returned when a resource is not yet available
	// but it might be in the future.
	NotYetAvailable = jujuerrors.NotYetAvailable
)
