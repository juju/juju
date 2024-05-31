// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	application "github.com/juju/juju/domain/application/modelmigration"
	blockdevice "github.com/juju/juju/domain/blockdevice/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	lease "github.com/juju/juju/domain/lease/modelmigration"
	machine "github.com/juju/juju/domain/machine/modelmigration"
	model "github.com/juju/juju/domain/model/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	network "github.com/juju/juju/domain/network/modelmigration"
	storage "github.com/juju/juju/domain/storage/modelmigration"
	internalstorage "github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// ImportOperations registers the import operations with the given coordinator.
// This is a convenience function that can be used by the main migration package
// to register all the import operations.
func ImportOperations(
	coordinator Coordinator,
	logger logger.Logger,
	modelDefaultsProvider modelconfigservice.ModelDefaultsProvider,
	registry internalstorage.ProviderRegistry,
) {
	// Note: All the import operations are registered here.
	// Order is important!
	lease.RegisterImport(coordinator, logger.Child("lease"))
	externalcontroller.RegisterImport(coordinator)
	credential.RegisterImport(coordinator, logger.Child("credential"))
	model.RegisterImport(coordinator, logger.Child("model"))
	modelconfig.RegisterImport(coordinator, modelDefaultsProvider, logger.Child("modelconfig"))
	network.RegisterImport(coordinator, logger.Child("network"))
	machine.RegisterImport(coordinator, logger.Child("machine"))
	application.RegisterImport(coordinator, registry, logger.Child("application"))
	blockdevice.RegisterImport(coordinator, logger.Child("blockdevice"))
	// TODO(storage) - we need to break out storage pools and import BEFORE applications.
	storage.RegisterImport(coordinator, registry, logger.Child("storage"))
}
