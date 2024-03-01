// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/errors"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, registry storage.ProviderRegistry) {
	coordinator.Add(&importOperation{registry: registry})
}

// ImportService provides a subset of the storage domain
// service methods needed for storage pool import.
type ImportService interface {
	CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs service.PoolAttrs) error
}

type importOperation struct {
	modelmigration.BaseOperation

	registry storage.ProviderRegistry
	service  ImportService
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB()), logger, i.registry)
	return nil
}

// Execute the import on the storage pools contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, pool := range model.StoragePools() {
		err := i.service.CreateStoragePool(ctx, pool.Name(), storage.ProviderType(pool.Provider()), pool.Attributes())
		if err != nil {
			return errors.Annotatef(err, "creating storage pool %q", pool.Name())
		}
	}
	return nil
}
