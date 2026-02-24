// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/internal/errors"
)

// Service provides the API for working with the network domain.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// ProviderService provides the API for working with network spaces.
type ProviderService struct {
	Service
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking]
	providerWithZones      providertracker.ProviderGetter[ProviderWithZones]
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(
	st State,
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking],
	providerWithZones providertracker.ProviderGetter[ProviderWithZones],
	logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		providerWithNetworking: providerWithNetworking,
		providerWithZones:      providerWithZones,
	}
}

// supportsNetworking reports if the provider supports networking.
// TODO (manadart 2026-02-17) This is a characteristic of the provider
// implementation, not of the provider's particular cloud.
// As such, it should be cached to avoid repeated calls to the provider.
func (s *ProviderService) supportsNetworking(ctx context.Context) (bool, error) {
	provider, err := s.providerWithNetworking(ctx)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return false, errors.Capture(err)
	}
	return provider != nil, nil
}
