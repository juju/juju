// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
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
	CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs service.PoolAttrs) error
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
	for _, pool := range model.StoragePools() {
		err := i.service.CreateStoragePool(ctx, pool.Name(), storage.ProviderType(pool.Provider()), pool.Attributes())
		if err != nil {
			return errors.Errorf("creating storage pool %q: %w", pool.Name(), err)
		}
	}
	return nil
}
