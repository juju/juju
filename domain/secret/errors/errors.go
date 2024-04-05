// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// SecretNotFound describes an error that occurs when the secret being operated on
	// does not exist.
	SecretNotFound = errors.ConstError("secret not found")

	// SecretAlreadyExists describes an error that occurs when the secret being operated on
	// already exists.
	SecretAlreadyExists = errors.ConstError("secret already exists")

	// SecretRevisionNotFound describes an error that occurs when the secret revision being operated on
	// does not exist.
	SecretRevisionNotFound = errors.ConstError("secret revision not found")
)
