// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/description/v5"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/modelmigration"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	internalstorage "github.com/juju/juju/internal/storage"
)

var logger = loggo.GetLogger("juju.migration.storagepools")

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, registry internalstorage.ProviderRegistry) {
	coordinator.Add(&exportOperation{registry: registry})
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

	registry internalstorage.ProviderRegistry
	service  ExportService
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ModelDB()), logger, e.registry)
	return nil
}

// Execute the export, adding the storage pools to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	poolConfigs, err := e.service.AllStoragePools(ctx)
	if err != nil {
		return errors.Annotate(err, "listing pools")
	}

	builtIn, err := domainstorage.BuiltInStoragePools()
	if err != nil {
		return errors.Trace(err)
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
