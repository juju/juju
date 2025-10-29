// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	StoragePoolState
	StorageState

	// GetStorageAttachmentUUIDForStorageInstanceAndUnit returns the
	// [domainstorageprovisioning.StorageAttachmentUUID] associated with the given
	// storage instance uuid and unit uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storage/errors.StorageInstanceNotFound]
	// if the storage instance for the supplied uuid no longer exists.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the
	// unit no longer exists for the supplied uuid.
	GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		context.Context,
		domainstorage.StorageInstanceUUID,
		coreunit.UUID,
	) (domainstorageprovisioning.StorageAttachmentUUID, error)

	// GetStorageInstanceAttachments returns the set of attachments a storage
	// instance has. If the storage instance has no attachments then an empty
	// slice.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storage/errors.StorageInstanceNotFound]
	// if the storage instance for the supplied uuid does not exist.
	GetStorageInstanceAttachments(
		context.Context,
		domainstorage.StorageInstanceUUID,
	) ([]domainstorageprovisioning.StorageAttachmentUUID, error)

	// GetStorageInstanceUUIDByID retrieves the UUID of a storage instance by
	// its ID.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.StorageInstanceNotFound] when no storage
	// instance exists for the provided ID.
	GetStorageInstanceUUIDByID(
		ctx context.Context, storageID string,
	) (domainstorage.StorageInstanceUUID, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	*StoragePoolService
	*StorageService

	logger logger.Logger
	st     State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(
	st State,
	logger logger.Logger,
	registryGetter corestorage.ModelStorageRegistryGetter,
) *Service {
	return &Service{
		StoragePoolService: &StoragePoolService{
			st:             st,
			registryGetter: registryGetter,
		},
		StorageService: &StorageService{
			st:             st,
			registryGetter: registryGetter,
		},
		logger: logger,
		st:     st,
	}
}

// GetStorageRegistry returns the storage registry for the model.
//
// Deprecated: This method will be removed once the storage registry is fully
// implemented in each service.
func (s *Service) GetStorageRegistry(ctx context.Context) (internalstorage.ProviderRegistry, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	registry, err := s.StorageService.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf("getting storage registry: %w", err)
	}
	return registry, nil
}
