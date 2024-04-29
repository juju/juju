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
	// SkipPing is specified to skip pinging the backend.
	SkipPing bool
	// Reset is a list of configs to reset.
	Reset []string
}

// DeleteSecretBackendParams is used to delete a secret backend.
type DeleteSecretBackendParams struct {
	secretbackend.BackendIdentifier
	// DeleteInUse is specified to delete the backend even if it is in use.
	DeleteInUse bool
}

// RevisionInfo is used to hold info about an external secret revision.
type RevisionInfo struct {
	Revision int
	ValueRef *coresecrets.ValueRef
}
