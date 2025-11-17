// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AlreadyActivated describes an error that occurs when an attempt is made
	// to activate a model that has already been activated.
	AlreadyActivated = errors.ConstError("model already activated")

	// AlreadyExists describes an error that occurs when a model already exists.
	AlreadyExists = errors.ConstError("model already exists")

	// ModelNameNotValid describes an error where the supplied model name is not
	// valid.
	ModelNameNotValid = errors.ConstError("model name not valid")
)
