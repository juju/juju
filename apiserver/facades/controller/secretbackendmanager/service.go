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
	WatchSecretBackendRotationChanges(context.Context) (watcher.SecretBackendRotateWatcher, error)
}
