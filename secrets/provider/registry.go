// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
)

type secretStoreRegistry struct {
	stores map[string]SecretStoreProvider
}

var globalStoreRegistry = &secretStoreRegistry{
	stores: map[string]SecretStoreProvider{},
}

// Register registers the named secret store provider.
func (r *secretStoreRegistry) Register(storeType string, p SecretStoreProvider) error {
	if r.stores[storeType] != nil {
		return errors.Errorf("duplicate store name %q", storeType)
	}
	r.stores[storeType] = p
	return nil
}

// Provider returns the named secret store provider.
func (r *secretStoreRegistry) Provider(storeType string) (SecretStoreProvider, error) {
	p, ok := r.stores[storeType]
	if !ok {
		return nil, errors.NewNotFound(
			nil, fmt.Sprintf("no registered provider for %q", storeType),
		)
	}
	return p, nil
}

// Register registers the named secret store provider.
func Register(name string, p SecretStoreProvider) {
	if err := globalStoreRegistry.Register(name, p); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
}

// Provider returns the named secret store provider.
func Provider(storeType string) (SecretStoreProvider, error) {
	return globalStoreRegistry.Provider(storeType)
}
