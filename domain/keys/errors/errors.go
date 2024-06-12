// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// AuthorisedKeyAlreadyExists indicates that the authorised key already
	// exists for the specified user.
	AuthorisedKeyAlreadyExists = errors.ConstError("authorised key already exists")

	// InvalidAuthorisedKey indicates a problem with an authorised key where it
	// was unable to be understood.
	InvalidAuthorisedKey = errors.ConstError("invalid authorised key")

	// ReservedCommentViolation indicates that a key contains a comment that is
	// reserved within the Juju system and cannot be used.
	ReservedCommentViolation = errors.ConstError("key contains a reserved comment")
)
