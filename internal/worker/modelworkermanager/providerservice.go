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
	// Model returns information for the current model
	Model(ctx context.Context) (coremodel.ModelInfo, error)
	// WatchModelCloudCredential returns a new NotifyWatcher watching for changes that
	// result in the cloud spec for a model changing.
	WatchModelCloudCredential(ctx context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error)
	// WatchModel returns a watcher that emits an event if the model changes.
	WatchModel(ctx context.Context) (watcher.NotifyWatcher, error)
}

// ProviderCloudService represents the cloud service provided by the provider.
type ProviderCloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

// ProviderConfigService represents the config service provided by the provider.
type ProviderConfigService interface {
	// ModelConfig returns the model configuration for the given tag.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that observes changes to the specified
	// model configuration.
	Watch(context.Context) (watcher.StringsWatcher, error)
}

// ProviderCredentialService represents the credential service provided by the
// provider.
type ProviderCredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	// InvalidateCredential marks the cloud credential for the given key as invalid.
	InvalidateCredential(ctx context.Context, key credential.Key, reason string) error
}

// ProviderServices provides access to the services required by the
// provider.
type ProviderServices interface {
	// Model returns the provider model service.
	Model() ProviderModelService
	// Cloud returns the cloud service.
	Cloud() ProviderCloudService
	// Config returns the config service.
	Config() ProviderConfigService
	// Credential returns the credential service.
	Credential() ProviderCredentialService
}

// ProviderServicesGetter represents a way to get a ProviderServices
// for a given model.
type ProviderServicesGetter interface {
	// ServicesForModel returns a ProviderServices for the given model.
	ServicesForModel(modelUUID string) ProviderServices
}
