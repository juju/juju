// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	access "github.com/juju/juju/domain/access/modelmigration"
	agentpassword "github.com/juju/juju/domain/agentpassword/modelmigration"
	application "github.com/juju/juju/domain/application/modelmigration"
	blockcommand "github.com/juju/juju/domain/blockcommand/modelmigration"
	blockdevice "github.com/juju/juju/domain/blockdevice/modelmigration"
	cloudimagemetadata "github.com/juju/juju/domain/cloudimagemetadata/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	crossmodelrelation "github.com/juju/juju/domain/crossmodelrelation/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	keymanager "github.com/juju/juju/domain/keymanager/modelmigration"
	lease "github.com/juju/juju/domain/lease/modelmigration"
	machine "github.com/juju/juju/domain/machine/modelmigration"
	model "github.com/juju/juju/domain/model/modelmigration"
	modelagent "github.com/juju/juju/domain/modelagent/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	network "github.com/juju/juju/domain/network/modelmigration"
	operation "github.com/juju/juju/domain/operation/modelmigration"
	port "github.com/juju/juju/domain/port/modelmigration"
	relation "github.com/juju/juju/domain/relation/modelmigration"
	resource "github.com/juju/juju/domain/resource/modelmigration"
	secret "github.com/juju/juju/domain/secret/modelmigration"
	sequence "github.com/juju/juju/domain/sequence/modelmigration"
	status "github.com/juju/juju/domain/status/modelmigration"
	storage "github.com/juju/juju/domain/storage/modelmigration"
	unitstate "github.com/juju/juju/domain/unitstate/modelmigration"
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
	clock clock.Clock,
	logger logger.Logger,
) {
	// Note: All the import operations are registered here.
	// Order is important!

	// The domain services are not available until the model has been
	// imported and activated. If you require the domain services, you must
	// not access them directly, instead provide a way to access them in
	// a lazy fashion.

	lease.RegisterImport(coordinator, logger.Child("lease"))
	externalcontroller.RegisterImport(coordinator)
	credential.RegisterImport(coordinator, logger.Child("credential"))
	model.RegisterModelImport(coordinator, clock, logger.Child("model"))

	// Domain services is available for all the following services, but only
	// after the model has been imported and activated.

	sequence.RegisterImport(coordinator)
	keymanager.RegisterImport(coordinator, clock, logger.Child("keymanager"))
	modelconfig.RegisterImport(coordinator, modelDefaultsProvider, logger.Child("modelconfig"))
	access.RegisterImport(coordinator, clock, logger.Child("access"))
	network.RegisterImportSubnets(coordinator, logger.Child("subnets"))
	machine.RegisterImport(coordinator, clock, logger.Child("machine"))
	network.RegisterLinkLayerDevicesImport(coordinator, logger.Child("linklayerdevices"))
	application.RegisterImport(coordinator, clock, logger.Child("application"))
	// BlockDevice requires machines to be imported first.
	blockdevice.RegisterImport(coordinator, logger.Child("blockdevice"))
	// Storage requires machines and units (via the application domain) to be
	// imported first. Volumes require block devices to be imported first.
	storage.RegisterImport(coordinator, storageRegistryGetter, logger.Child("storage"))
	network.RegisterImportCloudService(coordinator, logger.Child("cloudservice"))
	agentpassword.RegisterImport(coordinator)
	crossmodelrelation.RegisterImport(coordinator, clock, logger.Child("crossmodelrelation"))
	relation.RegisterImport(coordinator, clock, logger.Child("relation"))
	access.RegisterOfferAccessImport(coordinator, clock, logger.Child("offeraccess"))
	status.RegisterImport(coordinator, clock, logger.Child("status"))
	resource.RegisterImport(coordinator, clock, logger.Child("resource"))
	port.RegisterImport(coordinator, logger.Child("port"))
	secret.RegisterImport(coordinator, logger.Child("secret"))
	cloudimagemetadata.RegisterImport(coordinator, logger.Child("cloudimagemetadata"), clock)
	unitstate.RegisterImport(coordinator, logger.Child("unitstate"))
	operation.RegisterImport(coordinator, clock, logger.Child("operation"))

	// model agent must come after machine and unit
	modelagent.RegisterImport(coordinator, logger.Child("modelagent"))

	// Block command is probably best processed last, is that will prevent
	// any block commands from being executed before all the other operations
	// have been completed.
	blockcommand.RegisterImport(coordinator, logger.Child("blockcommand"))

	// Finally, we need to activate the model after all other operations
	// have been completed.
	model.RegisterModelActivationImport(coordinator, logger.Child("model"))
}
