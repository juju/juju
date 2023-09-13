// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// AlreadyExists describes an error that occurs when a model already exists.
	AlreadyExists = errors.ConstError("model already exists")

	// NotFound describes an error that occurs when the model being operated on
	// does not exist.
	NotFound = errors.ConstError("model not found")
)
