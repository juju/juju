// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/juju/core/modelmigration"
	status "github.com/juju/juju/core/status"
	application "github.com/juju/juju/domain/application/modelmigration"
	blockdevice "github.com/juju/juju/domain/blockdevice/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	lease "github.com/juju/juju/domain/lease/modelmigration"
	machine "github.com/juju/juju/domain/machine/modelmigration"
	model "github.com/juju/juju/domain/model/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	storage "github.com/juju/juju/domain/storage/modelmigration"
	internalstorage "github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// Logger is the interface that is used to log messages.
type Logger interface {
	Infof(string, ...any)
	Debugf(string, ...any)
}

// ImportOperations registers the import operations with the given coordinator.
// This is a convenience function that can be used by the main migration package
// to register all the import operations.
func ImportOperations(
	coordinator Coordinator,
	logger Logger,
	modelDefaultsProvider modelconfigservice.ModelDefaultsProvider,
	registry internalstorage.ProviderRegistry,
	statusHistory status.StatusHistoryForModel,
) {
	// Note: All the import operations are registered here.
	// Order is important!
	lease.RegisterImport(coordinator, logger)
	externalcontroller.RegisterImport(coordinator)
	credential.RegisterImport(coordinator)
	model.RegisterImport(coordinator, logger)
	modelconfig.RegisterImport(coordinator, modelDefaultsProvider)
	machine.RegisterImport(coordinator)
	application.RegisterImport(coordinator, registry, statusHistory)
	blockdevice.RegisterImport(coordinator)
	// TODO(storage) - we need to break out storage pools and import BEFORE applications.
	storage.RegisterImport(coordinator, registry)
}
