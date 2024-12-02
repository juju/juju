// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// UserNotFound describes an error that occurs when the user being requested does
	// not exist.
	UserNotFound = errors.ConstError("user not found")

	// UserCreatorUUIDNotFound describes an error that occurs when a user's
	// creator UUID, the user that created the user in question, does not exist.
	UserCreatorUUIDNotFound = errors.ConstError("user creator UUID not found")

	// UserNameNotValid describes an error that occurs when a supplied username
	// is not valid.
	// Examples of this include illegal characters or usernames that are not of
	// sufficient length.
	UserNameNotValid = errors.ConstError("username not valid")

	// UserUUIDNotValid describes an error that occurs when a supplied UUID is
	// not valid.
	UserUUIDNotValid = errors.ConstError("User UUID not valid")

	// UserAlreadyExists describes an error that occurs when the user being
	// created already exists.
	UserAlreadyExists = errors.ConstError("user already exists")

	// UserAuthenticationDisabled describes an error that occurs when the user's
	// authentication mechanisms are disabled.
	UserAuthenticationDisabled = errors.ConstError("user authentication disabled")

	// UserUnauthorized describes an error that occurs when the user does not
	// have the required permissions to perform an action.
	UserUnauthorized = errors.ConstError("user unauthorized")

	// PermissionNotValid is used when a permission has failed validation.
	PermissionNotValid = errors.ConstError("permission not valid")

	// PermissionNotFound describes an error that occurs when the permission being
	// requested does not exist.
	PermissionNotFound = errors.ConstError("permission not found")

	// PermissionAlreadyExists describes an error that occurs when the user being
	// created already exists.
	PermissionAlreadyExists = errors.ConstError("permission already exists")

	// PermissionTargetInvalid describes an error that occurs when the target of the
	// permission is invalid.
	PermissionTargetInvalid = errors.ConstError("permission target invalid")

	// PermissionAccessInvalid describes an error that occurs when the access of the
	// permission is invalid for the given target.
	PermissionAccessInvalid = errors.ConstError("permission access invalid")

	// PermissionAccessGreater describes an error that occurs when current access of
	// the user is greater or equal to the access being granted.
	PermissionAccessGreater = errors.ConstError("access or greater")

	// ActivationKeyNotFound describes an error that occurs when the
	// activation key is not found.
	ActivationKeyNotFound = errors.ConstError("activation key not found")

	// ActivationKeyNotValid describes an error that occurs when the
	// activation key is not valid.
	ActivationKeyNotValid = errors.ConstError("activation key not valid")

	// UniqueIdentifierIsNotUnique describes an error that occurs when a unique
	// identifier is found in multiple places as an identifier. E.G. Model UUID is
	// found as an Offer UUID.
	UniqueIdentifierIsNotUnique = errors.ConstError("unique identifier is not unique")

	// AccessNotFound describes an error that occurs no access is found for a
	// user on a target.
	AccessNotFound = errors.ConstError("access not found")

	// UserNeverAccessedModel describes an error that occurs if a user has
	// never accessed a model.
	UserNeverAccessedModel = errors.ConstError("user never accessed model")
)
