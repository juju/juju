// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ProviderModelService represents the model service provided by the provider.
type ProviderModelService interface {
	// Model returns the read-only default model.
	Model(ctx context.Context) (coremodel.ReadOnlyModel, error)
}

// ProviderCloudService represents the cloud service provided by the provider.
type ProviderCloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// WatchCloud returns a watcher that observes changes to the specified cloud.
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

// ProviderConfigService represents the config service provided by the provider.
type ProviderConfigService interface {
	// ModelConfig returns the model configuration for the given tag.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// WatchModelConfig returns a watcher that observes changes to the specified
	// model configuration.
	Watch() (watcher.StringsWatcher, error)
}

// ProviderCredentialService represents the credential service provided by the
// provider.
type ProviderCredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// ProviderServiceFactory provides access to the services required by the
// provider.
type ProviderServiceFactory interface {
	// Model returns the provider model service.
	Model() ProviderModelService
	// Cloud returns the cloud service.
	Cloud() ProviderCloudService
	// Config returns the config service.
	Config() ProviderConfigService
	// Credential returns the credential service.
	Credential() ProviderCredentialService
}

// ProviderServiceFactoryGetter represents a way to get a ProviderServiceFactory
// for a given model.
type ProviderServiceFactoryGetter interface {
	// FactoryForModel returns a ProviderServiceFactory for the given model.
	FactoryForModel(modelUUID string) ProviderServiceFactory
}
