// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// UpsertSecretBackendParams are used to upsert a secret backend.
type UpsertSecretBackendParams struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]interface{}
}

// SecretBackendInfo contains information about a secret backend.
type SecretBackendInfo struct {
	secrets.SecretBackend

	NumSecrets int
	Status     string
	Message    string
}
