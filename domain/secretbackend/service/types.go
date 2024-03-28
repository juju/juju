// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
)

// SecretBackendInfo contains information about a secret backend.
type SecretBackendInfo struct {
	coresecrets.SecretBackend

	NumSecrets int
	Status     string
	Message    string
}

// UpdateSecretBackendParams is used to update a secret backend.
type UpdateSecretBackendParams struct {
	secretbackend.UpdateSecretBackendParams
	// Force is used to force the update without validation.
	Force bool
	// Reset is a list of configs to reset.
	Reset []string
}
