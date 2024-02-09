// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
)

var logger = loggo.GetLogger("juju.environs")

// ProviderRegistry is an interface that provides methods for registering
// and obtaining environment providers by provider name.
type ProviderRegistry interface {
	// RegisterProvider registers a new environment provider with the given
	// name, and zero or more aliases. If a provider already exists with the
	// given name or alias, an error will be returned.
	RegisterProvider(p EnvironProvider, providerType string, providerTypeAliases ...string) error

	// UnregisterProvider unregisters the environment provider with the given name.
	UnregisterProvider(providerType string)

	// RegisteredProviders returns the names of the registered environment
	// providers.
	RegisteredProviders() []string

	// Provider returns the environment provider with the specified name. If no
	// provider has been registered with the supplied name then an error
	// satisfying errors.NotFound is returned.
	Provider(providerType string) (EnvironProvider, error)
}

// GlobalProviderRegistry returns the global provider registry.
func GlobalProviderRegistry() ProviderRegistry {
	return globalProviders
}

type globalProviderRegistry struct {
	// providers maps from provider type to EnvironProvider for
	// each registered provider type.
	providers map[string]EnvironProvider
	// providerAliases is a map of provider type aliases.
	aliases map[string]string
}

var globalProviders = &globalProviderRegistry{
	providers: map[string]EnvironProvider{},
	aliases:   map[string]string{},
}

func (r *globalProviderRegistry) RegisterProvider(p EnvironProvider, providerType string, providerTypeAliases ...string) error {
	if r.providers[providerType] != nil || r.aliases[providerType] != "" {
		return errors.Errorf("duplicate provider name %q", providerType)
	}
	r.providers[providerType] = p
	for _, alias := range providerTypeAliases {
		if r.providers[alias] != nil || r.aliases[alias] != "" {
			return errors.Errorf("duplicate provider alias %q", alias)
		}
		r.aliases[alias] = providerType
	}
	return nil
}

// UnregisterProvider removes the named provider from the list of available providers.
func (r *globalProviderRegistry) UnregisterProvider(providerType string) {
	delete(r.providers, providerType)
	for a, p := range r.aliases {
		if p == providerType {
			delete(r.aliases, a)
		}
	}
}

func (r *globalProviderRegistry) RegisteredProviders() []string {
	var p []string
	for k := range r.providers {
		p = append(p, k)
	}
	return p
}

// Provider implements ProviderRegistry.Provider()
func (r *globalProviderRegistry) Provider(providerType string) (EnvironProvider, error) {
	if alias, ok := r.aliases[providerType]; ok {
		providerType = alias
	}
	p, ok := r.providers[providerType]
	if !ok {
		return nil, errors.NewNotFound(
			nil, fmt.Sprintf("no registered provider for %q", providerType),
		)
	}
	return p, nil
}

// RegisterProvider registers a new environment provider. Name gives the name
// of the provider, and p the interface to that provider.
//
// RegisterProvider will panic if the provider name or any of the aliases
// are registered more than once.
// The return function can be used to unregister the provider and is used by tests.
func RegisterProvider(name string, p CloudEnvironProvider, alias ...string) (unregister func()) {
	if err := GlobalProviderRegistry().RegisterProvider(p, name, alias...); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
	return func() {
		GlobalProviderRegistry().UnregisterProvider(name)
	}
}

// RegisteredProviders enumerate all the environ providers which have been registered.
func RegisteredProviders() []string {
	return GlobalProviderRegistry().RegisteredProviders()
}

// Provider returns the previously registered provider with the given type.
func Provider(providerType string) (EnvironProvider, error) {
	return GlobalProviderRegistry().Provider(providerType)
}
