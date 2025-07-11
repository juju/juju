// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
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
