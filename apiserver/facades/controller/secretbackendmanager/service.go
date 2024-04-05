// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"

	"github.com/juju/juju/core/watcher"
)

type BackendService interface {
	RotateBackendToken(ctx context.Context, backendID string) error
	WatchSecretBackendRotationChanges() (watcher.SecretBackendRotateWatcher, error)
}
