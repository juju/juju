// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ModelService represents the model service provided by the provider.
type ModelService interface {
	// Model returns the read-only default model.
	Model(ctx context.Context) (model.ReadOnlyModel, error)
}

// CloudService represents the cloud service provided by the provider.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// WatchCloud returns a watcher that observes changes to the specified cloud.
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

// ConfigService represents the config service provided by the provider.
type ConfigService interface {
	// ModelConfig returns the model configuration for the given tag.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// WatchModelConfig returns a watcher that observes changes to the specified
	// model configuration.
	Watch() (watcher.StringsWatcher, error)
}

// CredentialService represents the credential service provided by the
// provider.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, id credential.Key) (cloud.Credential, error)
	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, id credential.Key) (watcher.NotifyWatcher, error)
}
