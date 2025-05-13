// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	StoragePoolState
	StorageState
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	*StoragePoolService
	*StorageService
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger, registryGetter corestorage.ModelStorageRegistryGetter) *Service {
	return &Service{
		StoragePoolService: &StoragePoolService{
			st:             st,
			logger:         logger,
			registryGetter: registryGetter,
		},
		StorageService: &StorageService{
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
func (s *Service) GetStorageRegistry(ctx context.Context) (_ internalstorage.ProviderRegistry, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	registry, err := s.StorageService.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf("getting storage registry: %w", err)
	}
	return registry, nil
}
