// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// ApplicationIDNotValid describes an error when the application ID is
	// not valid.
	ApplicationIDNotValid = errors.ConstError("application ID not valid")

	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ArgumentNotValid describes an error that occurs when an argument to
	// the service is invalid.
	ArgumentNotValid = errors.ConstError("argument not valid")

	// ResourceNotFound describes an error that occurs when a resource is
	// not found.
	ResourceNotFound = errors.ConstError("resource not found")

	// RetrievedByTypeNotValid describes an error where the retrieved by type is
	// neither user, unit nor application.
	RetrievedByTypeNotValid = errors.ConstError("retrieved by type not valid")

	// ResourceNameNotValid describes an error where the resource name is not
	// valid, usually because it's empty.
	ResourceNameNotValid = errors.ConstError("resource name not valid")

	// UnitNotFound describes an error that occurs when the unit being operated on
	// does not exist.
	UnitNotFound = errors.ConstError("unit not found")

	// UnitUUIDNotValid describes an error when the unit UUID is
	// not valid.
	UnitUUIDNotValid = errors.ConstError("unit UUID not valid")

	// ResourceStateNotValid describes an error where the resource state is not
	// valid.
	ResourceStateNotValid = errors.ConstError("resource state not valid")

	// CleanUpStateNotValid describes an error where the application state is
	// during cleanup. It means that application dependencies are deleted in
	// an incorrect order.
	CleanUpStateNotValid = errors.ConstError("cleanup state not valid")

	// StoredResourceNotFound describes an error where the stored resource
	// cannot be found in the relevant resource persistence layer for its
	// resource type.
	StoredResourceNotFound = errors.ConstError("stored resource not found")
)
