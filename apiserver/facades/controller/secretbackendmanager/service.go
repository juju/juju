// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"

	"github.com/juju/worker/v4"

	corewatcher "github.com/juju/juju/core/watcher"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// BackendService defines the service methods that the secret backend manager facade uses.
type BackendService interface {
	RotateBackendToken(ctx context.Context, backendID string) error
	WatchSecretBackendRotationChanges() (SecretBackendRotateWatcher, error)
}

type serviceShim struct {
	backendService *secretbackendservice.WatchableService
}

// RotateBackendToken rotates the token for the given backend.
func (s *serviceShim) RotateBackendToken(ctx context.Context, backendID string) error {
	return s.backendService.RotateBackendToken(ctx, backendID)
}

// WatchSecretBackendRotationChanges sets up a watcher to notify of changes to secret backend rotations.
func (s *serviceShim) WatchSecretBackendRotationChanges() (SecretBackendRotateWatcher, error) {
	return s.backendService.WatchSecretBackendRotationChanges()
}

// SecretBackendRotateWatcher defines a watcher for secret backend rotation changes.
type SecretBackendRotateWatcher interface {
	worker.Worker
	Changes() <-chan []corewatcher.SecretBackendRotateChange
}
