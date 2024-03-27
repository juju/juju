// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/errors"

const (
	// NotFound describes an error that occurs when the permission being
	// requested does not exist.
	NotFound = errors.ConstError("permission not found")

	// AlreadyExists describes an error that occurs when the user being
	// created already exists.
	AlreadyExists = errors.ConstError("permission already exists")

	// TargetInvalid describes an error that occurs when the target of the
	// permission is invalid.
	TargetInvalid = errors.ConstError("permission target invalid")

	// UniqueIdentifierIsNotUnique describes an error that occurs when a unique
	// identifier is found in multiple places as an identifier. E.G. Model UUID is
	// found as an Offer UUID.
	UniqueIdentifierIsNotUnique = errors.ConstError("unique identifier is not unique")
)
