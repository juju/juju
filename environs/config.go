// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.environs")

// ProviderRegistry is an interface that provides methods for registering
// and obtaining environment providers by provider name.
type ProviderRegistry struct {
	mu sync.Mutex
	// providers maps from provider type to EnvironProvider for
	// each registered provider type.
	providers map[string]EnvironProvider
	// providerAliases is a map of provider type aliases.
	aliases map[string]string
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: map[string]EnvironProvider{},
		aliases:   map[string]string{},
	}
}

// Register registers a new environment provider with the given
// name, and zero or more aliases. If a provider already exists with the
// given name or alias, an error will be returned.
func (r *ProviderRegistry) Register(p EnvironProvider, providerType string, providerTypeAliases ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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

// Unregister unregisters the environment provider with the given name.
func (r *ProviderRegistry) Unregister(providerType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, providerType)
	for a, p := range r.aliases {
		if p == providerType {
			delete(r.aliases, a)
		}
	}
}

// RegisteredNames returns the names of the registered environment
// providers.
func (r *ProviderRegistry) RegisteredNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var p []string
	for k := range r.providers {
		p = append(p, k)
	}
	return p
}

// Provider returns the environment provider with the specified name.
func (r *ProviderRegistry) Provider(providerType string) (EnvironProvider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
func RegisterProvider(name string, p EnvironProvider, alias ...string) {
	providers := GlobalRegistry().Providers()
	if err := providers.Register(p, name, alias...); err != nil {
		panic(fmt.Errorf("juju: %v", err))
	}
}

// UnregisterProvider removes the provider with the given
// name from the registry.
func UnregisterProvider(name string) {
	GlobalRegistry().Providers().Unregister(name)
}

// RegisteredProviders enumerate all the environ providers which have been registered.
func RegisteredProviders() []string {
	return GlobalRegistry().Providers().RegisteredNames()
}

// Provider returns the previously registered provider with the given type.
func Provider(providerType string) (EnvironProvider, error) {
	return GlobalRegistry().Providers().Provider(providerType)
}
