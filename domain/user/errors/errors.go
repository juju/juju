// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// NotFound describes an error that occurs when the user being requested does
	// not exist.
	NotFound = errors.ConstError("user not found")

	// CreatorUUIDNotFound describes an error that occurs when a user's creator UUID,
	// the user that created the user in question, does not exist.
	CreatorUUIDNotFound = errors.ConstError("user creator UUID not found")

	// UserNameNotValid describes an error that occurs when a supplied username
	// is not valid.
	// Examples of this include illegal characters or usernames that are not of
	// sufficient length.
	UserNameNotValid = errors.ConstError("username not valid")

	// UUIDNotValid describes an error that occurs when a supplied UUID is not
	// valid.
	UUIDNotValid = errors.ConstError("User UUID not valid")

	// AlreadyExists describes an error that occurs when the user being
	// created already exists.
	AlreadyExists = errors.ConstError("user already exists")

	// AuthenticationDisabled describes an error that occurs when the user's
	// authentication mechanisms are disabled.
	AuthenticationDisabled = errors.ConstError("user authentication disabled")

	// Unauthorized describes an error that occurs when the user does not have
	// the required permissions to perform an action.
	Unauthorized = errors.ConstError("user unauthorized")

	// ActivationKeyNotFound describes an error that occurs when the activation
	// key is not found.
	ActivationKeyNotFound = errors.ConstError("activation key not found")

	// ActivationKeyNotValid describes an error that occurs when the activation
	// key is not valid.
	ActivationKeyNotValid = errors.ConstError("activation key not valid")

	// PermissionNotValid is used when a permission has failed validation.
	PermissionNotValid = errors.ConstError("permission not valid")
)
