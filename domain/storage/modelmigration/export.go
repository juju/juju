// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, storageRegistryGetter corestorage.ModelStorageRegistryGetter, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		storageRegistryGetter: storageRegistryGetter,
		logger:                logger,
	})
}

// ExportService provides a subset of the storage domain
// service methods needed for storage pool export.
type ExportService interface {
	AllStoragePools(ctx context.Context) ([]*internalstorage.Config, error)
}

// exportOperation describes a way to execute a migration for
// exporting storage pools.
type exportOperation struct {
	modelmigration.BaseOperation

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	service               ExportService
	logger                logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export storage"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ModelDB()), e.logger, e.storageRegistryGetter)
	return nil
}

// Execute the export, adding the storage pools to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	poolConfigs, err := e.service.AllStoragePools(ctx)
	if err != nil {
		return errors.Errorf("listing pools: %w", err)
	}

	builtIn, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return errors.Capture(err)
	}
	builtInNames := set.Strings{}
	for _, p := range builtIn {
		builtInNames.Add(p.Name)
	}

	for _, cfg := range poolConfigs {
		// We don't want to export built in providers, eg loop, rootfs, tmpfs.
		if builtInNames.Contains(cfg.Name()) {
			continue
		}
		model.AddStoragePool(description.StoragePoolArgs{
			Name:       cfg.Name(),
			Provider:   string(cfg.Provider()),
			Attributes: cfg.Attrs(),
		})
	}
	return nil
}
