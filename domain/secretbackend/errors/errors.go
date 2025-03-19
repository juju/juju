// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AlreadyExists describes an error that occurs when a secret backend already exists.
	AlreadyExists = errors.ConstError("secret backend already exists")

	// RefCountAlreadyExists describes an error that occurs when a secret backend reference count record already exists.
	RefCountAlreadyExists = errors.ConstError("secret backend reference count already exists")

	// RefCountNotFound describes an error that occurs when a secret backend reference count record is not found.
	RefCountNotFound = errors.ConstError("secret backend reference count not found")

	// NotFound describes an error that occurs when the secret backend being operated on does not exist.
	NotFound = errors.ConstError("secret backend not found")

	// NotValid describes an error that occurs when the secret backend being operated on is not valid.
	NotValid = errors.ConstError("secret backend not valid")

	// Forbidden describes an error that occurs when the operation is forbidden.
	Forbidden = errors.ConstError("secret backend operation forbidden")

	// NotSupported describes an error that occurs when the secret backend is not supported.
	NotSupported = errors.ConstError("secret backend not supported")
)
