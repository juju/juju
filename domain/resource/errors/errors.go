// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ApplicationIDNotValid describes an error when the application ID is
	// not valid.
	ApplicationIDNotValid = errors.ConstError("application ID not valid")

	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// CharmIDNotValid describes an error when the charm ID is
	// not valid.
	CharmIDNotValid = errors.ConstError("charm ID not valid")

	// ArgumentNotValid describes an error that occurs when an argument to
	// the service is invalid.
	ArgumentNotValid = errors.ConstError("argument not valid")

	// ResourceNotFound describes an error that occurs when a resource is
	// not found.
	ResourceNotFound = errors.ConstError("resource not found")

	// CharmmResourceNotFound describes an error that occurs when a charm
	// resource is not found.
	CharmResourceNotFound = errors.ConstError("charm resource not found")

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

	// ResourceRevisionNotValid describes an error where the resource revision
	// is not valid.
	ResourceRevisionNotValid = errors.ConstError("resource revision not valid")

	// StoredResourceAlreadyExists describes an error where the resource being
	// stored already exists in the store.
	StoredResourceAlreadyExists = errors.ConstError("stored resource already exists")

	// ResourceAlreadyStored describes an errors where the resource has already
	// been stored.
	ResourceAlreadyStored = errors.ConstError("resource already found in storage")

	// ApplicationNameNotValid describes an error when the application is
	// not valid.
	ApplicationNameNotValid = errors.ConstError("application name not valid")

	// ResourceUUIDNotValid describes an error when the resource UUID is
	// not valid.
	ResourceUUIDNotValid = errors.ConstError("resource UUID not valid")

	// OriginNotValid describes an error where the resource origin is invalid
	OriginNotValid = errors.ConstError("origin not valid")
)
