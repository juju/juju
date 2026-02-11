// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, storageRegistryGetter corestorage.ModelStorageRegistryGetter, logger logger.Logger) {
	coordinator.Add(&importOperation{
		storageRegistryGetter: storageRegistryGetter,
		logger:                logger,
	})
}

// ImportService provides a subset of the storage domain
// service methods needed for storage pool import.
type ImportService interface {
	// GetStoragePoolsToImport resolves the full set of storage pools to create during
	// model import.
	GetStoragePoolsToImport(ctx context.Context, userPools []description.StoragePool) (
		[]domainstorage.ImportStoragePoolParams,
		[]domainstorage.RecommendedStoragePoolParams,
		error,
	)
	// ImportStorageInstances creates new storage instances and storage unit
	// owners. Storage unit owners are created if the unit name is provided.
	ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error
	// ImportStoragePools creates new storage pools with the slice
	// of [domainstorage.ImportStoragePoolParams].
	ImportStoragePools(ctx context.Context, pools []domainstorage.ImportStoragePoolParams) error
	// SetRecommendedStoragePools persists the set of recommended storage pools
	// that are to be used for a model.
	SetRecommendedStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolParams) error
}

type importOperation struct {
	modelmigration.BaseOperation

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	service               ImportService
	logger                logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import storage"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB()), i.logger, i.storageRegistryGetter)
	return nil
}

// Execute the import on the storage pools contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	// TODO(adisazhar123): refactor opportunity. GetStoragePoolsToImport func
	// should just return the default pools and the merging / conflict resolution
	// with user pools happens in this import layer.
	poolsToImport, recommendedPools, err := i.service.GetStoragePoolsToImport(ctx, model.StoragePools())
	if err != nil {
		return errors.Errorf("getting pools to import: %w", err)
	}

	err = i.service.ImportStoragePools(ctx, poolsToImport)
	if err != nil {
		return errors.Errorf("importing storage pools %+v: %w", poolsToImport, err)
	}

	err = i.service.SetRecommendedStoragePools(ctx, recommendedPools)
	if err != nil {
		return errors.Errorf("setting recommended storage pools: %w", err)
	}

	if err := i.importStorageInstances(ctx, model.Storages()); err != nil {
		return errors.Errorf("importing storage instances: %w", err)
	}

	return nil
}

func (i *importOperation) importStorageInstances(ctx context.Context, instances []description.Storage) error {
	if instances == nil {
		return nil
	}

	args, err := transform.SliceOrErr(instances, func(in description.Storage) (domainstorage.ImportStorageInstanceParams, error) {
		if err := in.Validate(); err != nil {
			return domainstorage.ImportStorageInstanceParams{}, err
		}
		owner, _ := in.UnitOwner()
		var pool string
		var size uint64
		constraints, ok := in.Constraints()
		if ok {
			pool = constraints.Pool
			size = constraints.Size
		}
		return domainstorage.ImportStorageInstanceParams{
			StorageName:      in.Name(),
			StorageKind:      in.Kind(),
			StorageID:        in.ID(),
			UnitName:         owner,
			RequestedSizeMiB: size,
			PoolName:         pool,
		}, nil
	})

	if err != nil {
		return err
	}

	return i.service.ImportStorageInstances(ctx, args)
}
