// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
)

func init() {
	RegisterStore(juju.Store, juju.NewSecretStore)
}

// SecretsStoreFunc defines a function which creates a SecretsStore.
type SecretsStoreFunc func(cfg provider.StoreConfig) (provider.SecretsStore, error)

type secretStoreRegistry struct {
	stores map[string]SecretsStoreFunc
}

var globalStoreRegistry = &secretStoreRegistry{
	stores: map[string]SecretsStoreFunc{},
}

// RegisterStore registers the named secret store.
func (r *secretStoreRegistry) RegisterStore(storeType string, p SecretsStoreFunc) error {
	if r.stores[storeType] != nil {
		return errors.Errorf("duplicate store name %q", storeType)
	}
	r.stores[storeType] = p
	return nil
}

// Store returns the named secret store.
func (r *secretStoreRegistry) Store(storeType string) (SecretsStoreFunc, error) {
	p, ok := r.stores[storeType]
	if !ok {
		return nil, errors.NewNotFound(
			nil, fmt.Sprintf("no registered provider for %q", storeType),
		)
	}
	return p, nil
}

// RegisterStore registers the named secret store.
func RegisterStore(name string, p SecretsStoreFunc) {
	if err := globalStoreRegistry.RegisterStore(name, p); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
}

// Store returns the named secret store.
func Store(storeType string) (SecretsStoreFunc, error) {
	return globalStoreRegistry.Store(storeType)
}
