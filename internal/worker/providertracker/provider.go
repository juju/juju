// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/juju/caas"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// Provider is an interface that represents a provider, this can either be
// a CAAS broker or IAAS provider.
type Provider = providertracker.Provider

// ProviderConfigGetter is an interface that extends
// environs.EnvironConfigGetter to include the ControllerUUID method.
type ProviderConfigGetter interface {
	environs.EnvironConfigGetter

	// ControllerUUID returns the UUID of the controller.
	ControllerUUID() uuid.UUID

	// ForCredential returns a new cloned provider with the given
	// credential.
	ForCredential(jujucloud.Credential) ProviderConfigGetter
}

// IAASGetProvider creates a new provider from the given args.
func IAASGetProvider(newProvider IAASProviderFunc) func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
		// We can't use newProvider directly, as type invariance prevents us
		// from using it with the environs.GetEnvironAndCloud function.
		// Just wrap it in a closure to work around this.
		provider, spec, err := environs.GetEnvironAndCloud(ctx, getter, func(ctx context.Context, op environs.OpenParams) (environs.Environ, error) {
			return newProvider(ctx, op)
		})
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, err
		}
		return newForkableIAASProvider(provider, newProvider, getter), *spec, nil
	}
}

// CAASGetProvider creates a new provider from the given args.
func CAASGetProvider(newProvider CAASProviderFunc) func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
		cloudSpec, err := getter.CloudSpec(ctx)
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, errors.Errorf("cannot get cloud information: %w", err)
		}

		cfg, err := getter.ModelConfig(ctx)
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, err
		}

		broker, err := newProvider(ctx, environs.OpenParams{
			ControllerUUID: getter.ControllerUUID().String(),
			Cloud:          cloudSpec,
			Config:         cfg,
		})
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, errors.Errorf("cannot create caas broker: %w", err)
		}
		return newForkableCAASProvider(broker, newProvider, getter), cloudSpec, nil
	}
}

type forkableIAASProvider struct {
	environs.Environ
	newProvider  IAASProviderFunc
	configGetter ProviderConfigGetter
}

func newForkableIAASProvider(environ environs.Environ, newProvider IAASProviderFunc, getter ProviderConfigGetter) *forkableIAASProvider {
	return &forkableIAASProvider{
		Environ:      environ,
		newProvider:  newProvider,
		configGetter: getter,
	}
}

// ForCredential returns a new cloned forked provider with the given
// credential.
func (p *forkableIAASProvider) ForCredential(ctx context.Context, cred jujucloud.Credential) (Provider, error) {
	provider, _, err := environs.GetEnvironAndCloud(ctx, p.configGetter.ForCredential(cred), func(ctx context.Context, op environs.OpenParams) (environs.Environ, error) {
		return p.newProvider(ctx, op)
	})
	if err != nil {
		return nil, err
	}
	return newForkableIAASProvider(provider, p.newProvider, p.configGetter), nil
}

// EnvironProvider returns the underlying provider.
func (p *forkableIAASProvider) EnvironProvider() providertracker.EnvironProvider {
	return p.Environ
}

type forkableCAASProvider struct {
	caas.Broker
	newProvider  CAASProviderFunc
	configGetter ProviderConfigGetter
}

func newForkableCAASProvider(broker caas.Broker, newProvider CAASProviderFunc, getter ProviderConfigGetter) *forkableCAASProvider {
	return &forkableCAASProvider{
		Broker:       broker,
		newProvider:  newProvider,
		configGetter: getter,
	}
}

// ForCredential returns a new cloned forked provider with the given
// credential.
func (p *forkableCAASProvider) ForCredential(ctx context.Context, cred jujucloud.Credential) (Provider, error) {
	cloudSpec, err := p.configGetter.CloudSpec(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get cloud information: %w", err)
	}

	cfg, err := p.configGetter.ModelConfig(ctx)
	if err != nil {
		return nil, err
	}

	broker, err := p.newProvider(ctx, environs.OpenParams{
		ControllerUUID: p.configGetter.ControllerUUID().String(),
		Cloud:          cloudSpec,
		Config:         cfg,
	})
	if err != nil {
		return nil, errors.Errorf("cannot create caas broker: %w", err)
	}
	return newForkableCAASProvider(broker, p.newProvider, p.configGetter), nil
}

// EnvironProvider returns the underlying provider.
func (p *forkableCAASProvider) EnvironProvider() providertracker.EnvironProvider {
	return p.Broker
}

// environProvider returns the underlying provider.
type environProvider interface {
	// EnvironProvider returns the underlying provider.
	EnvironProvider() providertracker.EnvironProvider
}
