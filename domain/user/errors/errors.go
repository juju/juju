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

	// UserCreatorUUIDNotFound describes an error that occurs when a user's creator UUID,
	// the user that created the user in question, does not exist.
	UserCreatorUUIDNotFound = errors.ConstError("user creator UUID not found")

	// UsernameNotValid describes an error that occurs when a supplied username
	// is not valid.
	// Examples of this include illegal characters or usernames that are not of
	// sufficient length.
	UsernameNotValid = errors.ConstError("username not valid")

	// UserUUIDNotValid describes an error that occurs when a supplied UUID is not
	// valid.
	UserUUIDNotValid = errors.ConstError("User UUID not valid")

	// AlreadyExists describes an error that occurs when the user being
	// created already exists.
	AlreadyExists = errors.ConstError("user already exists")

	// UserAuthenticationDisabled describes an error that occurs when the users
	// authentication mechanisms are disabled.
	UserAuthenticationDisabled = errors.ConstError("user authentication disabled")

	// Unauthorized describes an error that occurs when the user does not have
	// the required permissions to perform an action.
	Unauthorized = errors.ConstError("user unauthorized")
)
