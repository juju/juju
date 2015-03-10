// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "github.com/juju/juju/storage"

// CommonProviders returns the storage providers used by all environments.
func CommonProviders() map[storage.ProviderType]storage.Provider {
	return map[storage.ProviderType]storage.Provider{
		LoopProviderType:   &loopProvider{logAndExec},
		RootfsProviderType: &rootfsProvider{logAndExec},
		TmpfsProviderType:  &tmpfsProvider{logAndExec},
	}
}
