// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"

	"github.com/juju/errors"
)

// deletedUserError is used to indicate when an attempt to mutate a deleted
// user is attempted.
type deletedUserError struct {
	userName string
}

func NewDeletedUserError(userName string) error {
	return &deletedUserError{userName: userName}
}

// Error implements the error interface.
func (e deletedUserError) Error() string {
	return fmt.Sprintf("user %q is permanently deleted", e.userName)
}

// IsDeletedUserError returns true if err is of type deletedUserError.
func IsDeletedUserError(err error) bool {
	_, ok := errors.Cause(err).(*deletedUserError)
	return ok
}

// neverLoggedInError is used to indicate that a user has never logged in.
type neverLoggedInError struct {
	userName string
}

func NewNeverLoggedInError(userName string) error {
	return &neverLoggedInError{userName: userName}
}

// Error returns the error string for a user who has never logged
// in.
func (e neverLoggedInError) Error() string {
	return `never logged in: "` + e.userName + `"`
}

// IsNeverLoggedInError returns true if err is of type neverLoggedInError.
func IsNeverLoggedInError(err error) bool {
	_, ok := errors.Cause(err).(*neverLoggedInError)
	return ok
}

// neverConnectedError is used to indicate that a user has never connected to
// an model.
type neverConnectedError struct {
	userName string
}

func NewNeverConnectedError(userName string) error {
	return &neverConnectedError{userName: userName}
}

// Error returns the error string for a user who has never connected to an
// model.
func (e neverConnectedError) Error() string {
	return `never connected: "` + e.userName + `"`
}

// IsNeverConnectedError returns true if err is of type neverConnectedError.
func IsNeverConnectedError(err error) bool {
	_, ok := errors.Cause(err).(*neverConnectedError)
	return ok
}
