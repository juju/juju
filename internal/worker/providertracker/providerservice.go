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

// DomainServicesGetter defines an interface that returns a DomainServices
// for a given model UUID.
type DomainServicesGetter interface {
	// ServicesForModel returns a ProviderServices for the given model.
	ServicesForModel(modelUUID string) DomainServices
}

// DomainServices provides access to the services required by the provider.
type DomainServices interface {
	// Model returns the model service.
	Model() ModelService
	// Cloud returns the cloud service.
	Cloud() CloudService
	// Config returns the config service.
	Config() ConfigService
	// Credential returns the credential service.
	Credential() CredentialService
}

// ModelService represents the model service provided by the provider.
type ModelService interface {
	// Model returns information for the current model.
	Model(ctx context.Context) (model.ModelInfo, error)
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
	// Watch returns a watcher that observes changes to the specified
	// model configuration.
	Watch() (watcher.StringsWatcher, error)
}

// CredentialService represents the credential service provided by the
// provider.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
	// InvalidateCredential marks the cloud credential for the given name, cloud, owner as invalid.
	InvalidateCredential(ctx context.Context, key credential.Key, reason string) error
}
