// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// ApplicationNotFound describes an error that occurs when the application being operated on
	// does not exist.
	ApplicationNotFound = errors.ConstError("application not found")
	// ApplicationHasUnits describes an error that occurs when the application being deleted still
	// has associated units.
	ApplicationHasUnits = errors.ConstError("application has units")
	// MissingStorageDirective describes an error that occurs when expected storage directives are missing.
	MissingStorageDirective = errors.ConstError("no storage directive specified")
)
