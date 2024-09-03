// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/providertracker"
	controllerproxyerrors "github.com/juju/juju/domain/controllerproxy/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/proxy"
)

// Provider represents an underlying cloud provider.
type Provider interface {
	environs.ConnectorInfo
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
	provider, err := s.providerGetter(ctx)
	if errors.Is(err, errors.NotSupported) {
		return nil, controllerproxyerrors.ProxyInfoNotSupported
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	proxier, err := provider.ConnectionProxyInfo(ctx)
	if errors.Is(err, errors.NotFound) {
		return nil, controllerproxyerrors.ProxyInfoNotFound
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return proxier, nil
}
