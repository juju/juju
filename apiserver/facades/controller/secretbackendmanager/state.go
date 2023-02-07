// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"time"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// BackendRotate instances provide secret backend watcher apis.
type BackendRotate interface {
	WatchSecretBackendRotationChanges() (state.SecretBackendRotateWatcher, error)
}

// BackendState instances provide secret backend apis.
type BackendState interface {
	GetSecretBackendByID(ID string) (*secrets.SecretBackend, error)
	UpdateSecretBackend(params state.UpdateSecretBackendParams) error
	SecretBackendRotated(ID string, next time.Time) error
}
