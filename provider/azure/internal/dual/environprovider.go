// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dual

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// EnvironProvider is an environs.EnvironProvider that switches between one of
// two environs.EnvironProviders depending on the configuration. Once a provider
// has delegated to one provider based on configuration, all future operations
// will be delegated to that same provider.
//
// One provider is considered the primary, and one the secondary. In case no
// provider has yet been selected based on configuration, the primary provider
// will take precedence for methods that do not receive configuration.
type EnvironProvider struct {
	primary, secondary environs.EnvironProvider
	isPrimary          func(*config.Config) bool

	mu     sync.Mutex
	active environs.EnvironProvider
}

// NewEnvironProvider returns a new EnvironProvider with the given primary and
// secondary EnvironProviders, and a function that determines whether or not
// the given configuration is relevant to the primary provider.
func NewEnvironProvider(
	primary, secondary environs.EnvironProvider,
	isPrimary func(*config.Config) bool,
) *EnvironProvider {
	return &EnvironProvider{
		primary:   primary,
		secondary: secondary,
		isPrimary: isPrimary,
	}
}

// PrepareForCreateEnvironment is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return p.ensureActive(cfg).PrepareForCreateEnvironment(cfg)
}

// PrepareForBootstrap is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	return p.ensureActive(cfg).PrepareForBootstrap(ctx, cfg)
}

// Open is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	return p.ensureActive(cfg).Open(cfg)
}

// Validate is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) Validate(newCfg, oldCfg *config.Config) (*config.Config, error) {
	if oldCfg != nil {
		oldPrimary := p.isPrimary(oldCfg)
		newPrimary := p.isPrimary(newCfg)
		if oldPrimary != newPrimary {
			return nil, errors.NotValidf("mixing primary and secondary configurations")
		}
	}
	return p.ensureActive(newCfg).Validate(newCfg, oldCfg)
}

// SecretAttrs is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	return p.ensureActive(cfg).SecretAttrs(cfg)
}

// BoilerplateConfig is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) BoilerplateConfig() string {
	return p.Active().BoilerplateConfig()
}

// RestrictedConfigAttributes is part of the environs.EnvironProvider interface.
func (p *EnvironProvider) RestrictedConfigAttributes() []string {
	return p.Active().RestrictedConfigAttributes()
}

// Active returns the currently active EnvironProvider.
func (p *EnvironProvider) Active() environs.EnvironProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active != nil {
		return p.active
	}
	return p.primary
}

// ensureActive returns the currently active EnvironProvider, setting it if
// it is not yet set based on the given configuration.
func (p *EnvironProvider) ensureActive(cfg *config.Config) environs.EnvironProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active == nil {
		if p.isPrimary(cfg) {
			p.active = p.primary
		} else {
			p.active = p.secondary
		}
	}
	return p.active
}
