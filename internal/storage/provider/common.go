// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/storage"
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
