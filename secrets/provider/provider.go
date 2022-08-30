// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

// SecretStoreProvider instances create secret stores.
type SecretStoreProvider interface {
	// TODO(wallyworld) - add config schema methods

	// NewStore returns a client which can access a secrets store.
	NewStore(cfg StoreConfig) (SecretsStore, error)
}
