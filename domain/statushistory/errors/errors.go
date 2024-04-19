// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// NotFound describes an error that occurs when the model being operated on
	// does not exist.
	NotFound = errors.ConstError("model not found")
)
