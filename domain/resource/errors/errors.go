// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ArgumentNotValid describes an error that occurs when an argument to
	// the service is invalid.
	ArgumentNotValid = errors.ConstError("argument not valid")

	// ResourceNotFound describes an error that occurs when a resource is
	// not found.
	ResourceNotFound = errors.ConstError("resource not found")

	// UnknownRetrievedByType describes an error where the retrieved by type is
	// neither user, unit nor application.
	UnknownRetrievedByType = errors.ConstError("unknown retrieved by type")

	// ResourceNameNotValid describes an error where the resource name is not
	// valid, usually because it's empty.
	ResourceNameNotValid = errors.ConstError("resource name not valid")

	// UnitNotFound describes an error that occurs when the unit being operated on
	// does not exist.
	UnitNotFound = errors.ConstError("unit not found")
)
