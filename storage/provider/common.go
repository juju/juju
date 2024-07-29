// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

var (
	errNoMountPoint = errors.New("filesystem mount point not specified")

	commonStorageProviders = map[storage.ProviderType]storage.Provider{
		LoopProviderType:   &loopProvider{logAndExec},
		RootfsProviderType: &rootfsProvider{logAndExec},
		TmpfsProviderType:  &tmpfsProvider{logAndExec},
	}
)

// CommonStorageProviders returns a storage.ProviderRegistry that contains
// the common storage providers.
func CommonStorageProviders() storage.ProviderRegistry {
	return storage.StaticProviderRegistry{Providers: commonStorageProviders}
}

// AllowedContainerProvider returns true if the specified storage type
// can be used with a vm container.
// Currently, this is a very restricted list, just the storage types
// created on disk or in memory.
// In future we'll need to look at supporting passthrough/bindmount storage.
func AllowedContainerProvider(providerType storage.ProviderType) bool {
	switch providerType {
	case LoopProviderType, RootfsProviderType, TmpfsProviderType:
		return true
	default:
		return false
	}
}
