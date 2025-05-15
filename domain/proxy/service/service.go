// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	proxyerrors "github.com/juju/juju/domain/proxy/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/proxy"
)

// Provider represents an underlying cloud provider.
type Provider interface {
	environs.ConnectorInfo

	// ProxyToApplication returns a proxy to the application with the given
	// port.
	ProxyToApplication(ctx context.Context, appName, remotePort string) (proxy.Proxier, error)
}

// Service defines a service for interacting with the underlying proxy provider.
type Service struct {
	providerGetter providertracker.ProviderGetter[Provider]
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(providerGetter providertracker.ProviderGetter[Provider]) *Service {
	return &Service{
		providerGetter: providerGetter,
	}
}

// GetConnectionProxyInfo returns the proxy information for the controller.
// If the provider does not support the environs.ConnectorInfo then it will
// return [errors.ProxyNotSupported], if the connection proxy info is not located
// it will return [errors.ProxyNotFound].
func (s *Service) GetConnectionProxyInfo(ctx context.Context) (proxy.Proxier, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, proxyerrors.ProxyInfoNotSupported
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	proxier, err := provider.ConnectionProxyInfo(ctx)
	if errors.Is(err, coreerrors.NotFound) {
		return nil, proxyerrors.ProxyInfoNotFound
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return proxier, nil
}

// GetProxyToApplication returns the proxy information for the application
// with the given port.
func (s *Service) GetProxyToApplication(ctx context.Context, appName, remotePort string) (proxy.Proxier, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, proxyerrors.ProxyNotSupported
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	return provider.ProxyToApplication(ctx, appName, remotePort)
}
