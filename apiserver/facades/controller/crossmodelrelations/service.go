// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// The following interfaces are used to access secret services.

type SecretService interface {
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch() (watcher.StringsWatcher, error)
}

type StatusService interface {
	// GetApplicationDisplayStatus returns the display status of the specified application.
	// The display status is equal to the application status if it is set, otherwise it is
	// derived from the unit display statuses.
	// If no application is found, an error satisfying [statuserrors.ApplicationNotFound]
	// is returned.
	GetApplicationDisplayStatus(context.Context, string) (status.StatusInfo, error)
}
