// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"sync"

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

	supportsNetworkingMu     sync.RWMutex
	cachedSupportsNetworking *bool
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
// We effectively ask this question of the provider only once,
// caching the result - it can never change during runtime.
func (s *ProviderService) supportsNetworking(ctx context.Context) (bool, error) {
	s.supportsNetworkingMu.RLock()
	if s.cachedSupportsNetworking != nil {
		defer s.supportsNetworkingMu.RUnlock()
		return *s.cachedSupportsNetworking, nil
	}
	s.supportsNetworkingMu.RUnlock()

	s.supportsNetworkingMu.Lock()
	defer s.supportsNetworkingMu.Unlock()

	// Re-check after taking the write lock in case another goroutine won the race.
	if s.cachedSupportsNetworking != nil {
		return *s.cachedSupportsNetworking, nil
	}

	provider, err := s.providerWithNetworking(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		supported := false
		s.cachedSupportsNetworking = &supported
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	supported := provider != nil
	s.cachedSupportsNetworking = &supported
	return supported, nil
}
