// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"

	watcher "github.com/juju/juju/core/watcher"
)

// BackendService defines the service methods that the secret backend manager facade uses.
type BackendService interface {
	RotateBackendToken(ctx context.Context, backendID string) error
	WatchSecretBackendRotationChanges() (watcher.SecretBackendRotateWatcher, error)
}

// type serviceShim struct {
// 	backendService *secretbackendservice.WatchableService
// }

// // RotateBackendToken rotates the token for the given backend.
// func (s *serviceShim) RotateBackendToken(ctx context.Context, backendID string) error {
// 	return s.backendService.RotateBackendToken(ctx, backendID)
// }

// // WatchSecretBackendRotationChanges sets up a watcher to notify of changes to secret backend rotations.
// func (s *serviceShim) WatchSecretBackendRotationChanges() (SecretBackendRotateWatcher, error) {
// 	return s.backendService.WatchSecretBackendRotationChanges()
// }

// // SecretBackendRotateWatcher defines a watcher for secret backend rotation changes.
// type SecretBackendRotateWatcher interface {
// 	worker.Worker
// 	Changes() <-chan []corewatcher.SecretBackendRotateChange
// }
