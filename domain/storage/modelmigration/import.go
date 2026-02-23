// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
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
	// ImportStoragePools creates new storage pools with the slice
	// of [domainstorage.ImportStoragePoolParams].
	ImportStoragePools(ctx context.Context, pools []domainstorage.UserStoragePoolParams) error

	// ImportStorageInstances imports storage instances and storage unit
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error

	// ImportFilesystems imports filesystems from the provided parameters.
	ImportFilesystems(ctx context.Context, args []domainstorage.ImportFilesystemParams) error
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
	if err := i.importStoragePools(ctx, model.StoragePools()); err != nil {
		return errors.Errorf("importing storage pools: %w", err)
	}

	if err := i.importStorageInstances(ctx, model.Storages()); err != nil {
		return errors.Errorf("importing storage instances: %w", err)
	}

	// Filesystems need to be handled differently for CAAS models. So until
	// this is implemented skip the import step.
	if model.Type() == coremodel.IAAS.String() {
		if err := i.importFilesystems(ctx, model.Filesystems()); err != nil {
			return errors.Errorf("importing filesystems: %w", err)
		}
	}

	return nil
}

func (i *importOperation) importStoragePools(ctx context.Context, pools []description.StoragePool) error {
	args := transform.Slice(pools, func(in description.StoragePool) domainstorage.UserStoragePoolParams {
		return domainstorage.UserStoragePoolParams{
			Name:       in.Name(),
			Provider:   in.Provider(),
			Attributes: in.Attributes(),
		}
	})
	return i.service.ImportStoragePools(ctx, args)
}

func (i *importOperation) importStorageInstances(ctx context.Context, instances []description.Storage) error {
	if len(instances) == 0 {
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

func (i *importOperation) importFilesystems(ctx context.Context, filesystems []description.Filesystem) error {
	if len(filesystems) == 0 {
		return nil
	}

	args := transform.Slice(filesystems, func(in description.Filesystem) domainstorage.ImportFilesystemParams {
		return domainstorage.ImportFilesystemParams{
			ID:                in.ID(),
			SizeInMiB:         in.Size(),
			ProviderID:        in.FilesystemID(),
			PoolName:          in.Pool(),
			StorageInstanceID: in.Storage(),
		}
	})

	return i.service.ImportFilesystems(ctx, args)
}
