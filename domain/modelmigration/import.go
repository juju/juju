// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/objectstore"
	corestorage "github.com/juju/juju/core/storage"
	access "github.com/juju/juju/domain/access/modelmigration"
	application "github.com/juju/juju/domain/application/modelmigration"
	blockcommand "github.com/juju/juju/domain/blockcommand/modelmigration"
	blockdevice "github.com/juju/juju/domain/blockdevice/modelmigration"
	cloudimagemetadata "github.com/juju/juju/domain/cloudimagemetadata/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	keymanager "github.com/juju/juju/domain/keymanager/modelmigration"
	lease "github.com/juju/juju/domain/lease/modelmigration"
	machine "github.com/juju/juju/domain/machine/modelmigration"
	model "github.com/juju/juju/domain/model/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	network "github.com/juju/juju/domain/network/modelmigration"
	port "github.com/juju/juju/domain/port/modelmigration"
	secret "github.com/juju/juju/domain/secret/modelmigration"
	storage "github.com/juju/juju/domain/storage/modelmigration"
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
	modelDefaultsProvider modelconfigservice.ModelDefaultsProvider,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
) {
	// Note: All the import operations are registered here.
	// Order is important!
	lease.RegisterImport(coordinator, logger.Child("lease"))
	externalcontroller.RegisterImport(coordinator)
	credential.RegisterImport(coordinator, logger.Child("credential"))
	model.RegisterImport(coordinator, logger.Child("model"))
	keymanager.RegisterImport(coordinator, logger.Child("keymanager"))
	modelconfig.RegisterImport(coordinator, modelDefaultsProvider)
	access.RegisterImport(coordinator, logger.Child("access"))
	network.RegisterImport(coordinator, logger.Child("network"))
	machine.RegisterImport(coordinator, logger.Child("machine"))
	application.RegisterImport(coordinator, storageRegistryGetter, objectStoreGetter, logger.Child("application"))
	port.RegisterImport(coordinator, logger.Child("port"))
	blockdevice.RegisterImport(coordinator, logger.Child("blockdevice"))
	// TODO(storage) - we need to break out storage pools and import BEFORE applications.
	storage.RegisterImport(coordinator, storageRegistryGetter, logger.Child("storage"))
	secret.RegisterImport(coordinator, logger.Child("secret"))
	cloudimagemetadata.RegisterImport(coordinator, logger.Child("cloudimagemetadata"), clock)

	// Block command is probably best processed last, is that will prevent
	// any block commands from being executed before all the other operations
	// have been completed.
	blockcommand.RegisterImport(coordinator, logger.Child("blockcommand"))
}
