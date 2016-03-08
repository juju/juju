// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.environs")

// disallowedWithNew holds those attributes
// that can not be set in an initial environment
// config used to bootstrap (they must only be set
// on a running environment where appropriate
// validation can be performed).
var disallowedWithBootstrap = []string{
	config.StorageDefaultBlockSourceKey,
}

// ProviderRegistry is an interface that provides methods for registering
// and obtaining environment providers by provider name.
type ProviderRegistry interface {
	// RegisterProvider registers a new environment provider with the given
	// name, and zero or more aliases. If a provider already exists with the
	// given name or alias, an error will be returned.
	RegisterProvider(p EnvironProvider, providerType string, providerTypeAliases ...string) error

	// RegisteredProviders returns the names of the registered environment
	// providers.
	RegisteredProviders() []string

	// Provider returns the environment provider with the specified name.
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

func (r *globalProviderRegistry) RegisteredProviders() []string {
	var p []string
	for k := range r.providers {
		p = append(p, k)
	}
	return p
}

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
func RegisterProvider(name string, p EnvironProvider, alias ...string) {
	if err := GlobalProviderRegistry().RegisterProvider(p, name, alias...); err != nil {
		panic(fmt.Errorf("juju: %v", err))
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

// BootstrapConfig returns a copy of the supplied configuration with the
// admin-secret and ca-private-key attributes removed. If the resulting
// config is not suitable for bootstrapping an environment, an error is
// returned.
func BootstrapConfig(cfg *config.Config) (*config.Config, error) {
	m := cfg.AllAttrs()
	// We never want to push admin-secret or the root CA private key to the cloud.
	delete(m, "admin-secret")
	delete(m, "ca-private-key")
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		return nil, err
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return nil, fmt.Errorf("model configuration has no agent-version")
	}
	return cfg, nil
}
