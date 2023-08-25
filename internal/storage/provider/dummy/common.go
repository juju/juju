// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import "github.com/juju/juju/internal/storage"

// StorageProviders returns a provider registry with some
// well-defined dummy storage providers.
func StorageProviders() storage.ProviderRegistry {
	return storage.StaticProviderRegistry{
		map[storage.ProviderType]storage.Provider{
			"static": &StorageProvider{IsDynamic: false},
			"modelscoped": &StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
			},
			"modelscoped-unreleasable": &StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: false,
			},
			"modelscoped-block": &StorageProvider{
				StorageScope: storage.ScopeEnviron,
				IsDynamic:    true,
				IsReleasable: true,
				SupportsFunc: func(k storage.StorageKind) bool {
					return k == storage.StorageKindBlock
				},
			},
			"machinescoped": &StorageProvider{
				StorageScope: storage.ScopeMachine,
				IsDynamic:    true,
			},
		},
	}
}
