// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// NotFound describes an error that occurs when the unit being operated on
	// does not exist.
	NotFound = errors.ConstError("unit not found")
	// NotAssigned describes an error that occurs when the unit being operated on
	// is not assigned.
	NotAssigned = errors.ConstError("unit not assigned")
)
