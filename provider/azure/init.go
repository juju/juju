// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/azure/internal/dual"
	legacyazure "github.com/juju/juju/provider/azure/internal/legacy/azure"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType                             = "azure"
	storageProviderType storage.ProviderType = "azure"
)

// NewProviders instantiates and returns Azure providers using the given
// configuration.
func NewProviders(config ProviderConfig) (environs.EnvironProvider, storage.Provider, error) {
	environProvider, err := NewEnvironProvider(config)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return environProvider, &azureStorageProvider{environProvider}, nil
}

func init() {
	environProvider, storageProvider, err := NewProviders(ProviderConfig{
		NewStorageClient:            azurestorage.NewClient,
		StorageAccountNameGenerator: RandomStorageAccountName,
	})
	if err != nil {
		panic(err)
	}

	legacyEnvironProvider := legacyazure.EnvironProvider()
	isAzureResourceManagerConfig := func(cfg *config.Config) bool {
		// The legacy environ provider will not be changed any more, so
		// if it's not valid for the legacy provider, assume it is valid
		// for the current provider.
		if _, err := legacyEnvironProvider.Validate(cfg, nil); err != nil {
			logger.Tracef(
				"config is not valid for legacy provider, assuming ARM: %v",
				err,
			)
			return true
		}
		return false
	}

	dualEnvironProvider := dual.NewEnvironProvider(
		environProvider, legacyEnvironProvider,
		isAzureResourceManagerConfig,
	)

	legacyStorageProvider := legacyazure.StorageProvider()
	dualStorageProvider := dual.NewStorageProvider(
		storageProvider, legacyStorageProvider,
		isAzureResourceManagerConfig,
	)

	environs.RegisterProvider(providerType, dualEnvironProvider)
	registry.RegisterProvider(storageProviderType, dualStorageProvider)
	registry.RegisterEnvironStorageProviders(providerType, storageProviderType)

	// TODO(axw) register an image metadata data source that queries
	// the Azure image registry, and introduce a way to disable the
	// common simplestreams source.
}
