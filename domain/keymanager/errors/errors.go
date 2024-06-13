// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// PublicKeyAlreadyExists indicates that the authorised key already
	// exists for the specified user.
	PublicKeyAlreadyExists = errors.ConstError("public key already exists")

	// InvalidPublicKey indicates a problem with a public key where it
	// was unable to be understood.
	InvalidPublicKey = errors.ConstError("invalid public key")

	// ReservedCommentViolation indicates that a key contains a comment that is
	// reserved within the Juju system and cannot be used.
	ReservedCommentViolation = errors.ConstError("key contains a reserved comment")
)
