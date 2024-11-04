// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
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
func NewService(st State, logger logger.Logger, registryGetter storage.ModelStorageRegistryGetter) *Service {
	return &Service{
		StoragePoolService: &StoragePoolService{
			st:             st,
			logger:         logger,
			registryGetter: registryGetter,
		},
	}
}

// GetStorageRegistry returns the storage registry for the model.
//
// Deprecated: This method will be removed once the storage registry is fully
// implemented in each service.
func (s *Service) GetStorageRegistry(ctx context.Context) (internalstorage.ProviderRegistry, error) {
	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf("getting storage registry: %w", err)
	}
	return registry, nil
}
