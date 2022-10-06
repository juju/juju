// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.secrets.provider")

type secretStoreRegistry struct {
	stores map[string]SecretStoreProvider
}

var globalStoreRegistry = &secretStoreRegistry{
	stores: map[string]SecretStoreProvider{},
}

// Register registers the named secret store provider.
func (r *secretStoreRegistry) Register(p SecretStoreProvider) error {
	storeType := p.Type()
	if r.stores[storeType] != nil {
		return errors.Errorf("duplicate store name %q", storeType)
	}
	logger.Tracef("registering secret provider %q", storeType)
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
func Register(p SecretStoreProvider) {
	if err := globalStoreRegistry.Register(p); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
}

// Provider returns the named secret store provider.
func Provider(storeType string) (SecretStoreProvider, error) {
	return globalStoreRegistry.Provider(storeType)
}
