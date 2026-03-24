// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/internal/errors"
)

// Service defines a service for interacting with the underlying state.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// LeadershipService provides the API for working with unit's state and
// persisting commit hook changes, including those that require leadership
// checks.
type LeadershipService struct {
	*Service
	leaderEnsurer          leadership.Ensurer
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking]
	logger                 logger.Logger
}

// NewLeadershipService returns a new LeadershipService for working with
// the underlying state.
func NewLeadershipService(
	st State,
	leaderEnsurer leadership.Ensurer,
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking],
	logger logger.Logger,
) *LeadershipService {
	return &LeadershipService{
		Service:                NewService(st, logger),
		leaderEnsurer:          leaderEnsurer,
		providerWithNetworking: providerWithNetworking,
		logger:                 logger,
	}
}

// supportsNetworking reports if the provider supports networking.
// TODO (manadart 2026-02-17) This is a characteristic of the provider
// implementation, not of the provider's particular cloud.
// As such, it should be cached to avoid repeated calls to the provider.
func (s *LeadershipService) supportsNetworking(ctx context.Context) (bool, error) {
	provider, err := s.providerWithNetworking(ctx)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return false, errors.Capture(err)
	}
	return provider != nil, nil
}
