// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// NotFound describes an error that occurs when the application being operated on
	// does not exist.
	NotFound = errors.ConstError("application not found")

	// HasUnits describes an error that occurs when the application being deleted still
	// has associated units.
	HasUnits = errors.ConstError("application has units")
)
