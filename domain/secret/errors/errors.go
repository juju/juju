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

	// SecretIsNotLocal describes an error that occurs when a secret is not from the current model.
	SecretIsNotLocal = errors.ConstError("secret is from a different model")

	// SecretLabelAlreadyExists describes an error that occurs when there's already a secret label for
	// a specified secret owner.
	SecretLabelAlreadyExists = errors.ConstError("secret label already exists")

	// SecretRevisionNotFound describes an error that occurs when the secret revision being operated on
	// does not exist.
	SecretRevisionNotFound = errors.ConstError("secret revision not found")

	// SecretConsumerNotFound describes an error that occurs when the secret consumer being operated on is not found.
	SecretConsumerNotFound = errors.ConstError("secret consumer not found")

	// AutoPruneNotSupported describes an error that occurs when a charm secret tries to set auto prune on a secret.
	AutoPruneNotSupported = errors.ConstError("charm secrets do not support auto prune")
)
