// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import "github.com/juju/juju/internal/storage"

// StorageProviders returns a provider registry with some
// well-defined dummy storage providers.
func StorageProviders() storage.ProviderRegistry {
	return storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
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
			// TODO (stickupkid): This shouldn't be here, but we're in the
			// processing of up-ending the storage provider registry. The
			// testing factory is very coupled to the fact it can just do
			// anything it wants at any moment.
			// For now hard code the k8s provider, we can fix once the testing
			// factory is removed.
			"kubernetes": &StorageProvider{},
		},
	}
}
