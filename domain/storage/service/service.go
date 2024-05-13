// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/storage"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	StoragePoolState
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	*StoragePoolService
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger, registry storage.ProviderRegistry) *Service {
	return &Service{
		StoragePoolService: &StoragePoolService{
			st:       st,
			logger:   logger,
			registry: registry,
		},
	}
}
