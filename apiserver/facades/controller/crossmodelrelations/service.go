// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
)

// The following interfaces are used to access secret services.

type SecretService interface {
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	RemoveRemoteSecretConsumer(ctx context.Context, unitName string) error
	WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error)
}
