// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	corestorage "github.com/juju/juju/core/storage"
	access "github.com/juju/juju/domain/access/modelmigration"
	agentpassword "github.com/juju/juju/domain/agentpassword/modelmigration"
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
	modelagent "github.com/juju/juju/domain/modelagent/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	network "github.com/juju/juju/domain/network/modelmigration"
	resource "github.com/juju/juju/domain/resource/modelmigration"
	secret "github.com/juju/juju/domain/secret/modelmigration"
	status "github.com/juju/juju/domain/status/modelmigration"
	storage "github.com/juju/juju/domain/storage/modelmigration"
	unitstate "github.com/juju/juju/domain/unitstate/modelmigration"
)

// Exporter defines the instance of the coordinator on which we'll register
// the export operations. A logger and a clock are needed for two of the
// export operations.
type Exporter struct {
	coordinator           Coordinator
	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	objectStoreGetter     objectstore.ModelObjectStoreGetter
	clock                 clock.Clock
	logger                logger.Logger
}

// NewExporter returns a new Exporter that encapsulates the
// legacyStateExporter. The legacyStateExporter is being deprecated, only
// needed until the migration to dqlite is complete.
func NewExporter(
	coordinator Coordinator,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
) *Exporter {
	return &Exporter{
		coordinator:       coordinator,
		objectStoreGetter: objectStoreGetter,
		clock:             clock,
		logger:            logger,
	}
}

// ExportOperations registers the export operations with the given coordinator.
// This is a convenience function that can be used by the main migration package
// to register all the export operations.
func (e *Exporter) ExportOperations(registry corestorage.ModelStorageRegistryGetter) {
	blockcommand.RegisterExport(e.coordinator, e.logger.Child("blockcommand"))
	modelconfig.RegisterExport(e.coordinator)
	access.RegisterExport(e.coordinator, e.logger.Child("access"))
	keymanager.RegisterExport(e.coordinator)
	credential.RegisterExport(e.coordinator, e.logger.Child("credential"))
	network.RegisterExport(e.coordinator, e.logger.Child("network"))
	externalcontroller.RegisterExport(e.coordinator)
	machine.RegisterExport(e.coordinator, e.clock, e.logger.Child("machine"))
	blockdevice.RegisterExport(e.coordinator, e.logger.Child("blockdevice"))
	storage.RegisterExport(e.coordinator, registry, e.logger.Child("storage"))
	secret.RegisterExport(e.coordinator, e.logger.Child("secret"))
	application.RegisterExport(e.coordinator, e.storageRegistryGetter, e.clock, e.logger.Child("application"))
	lease.RegisterExport(e.coordinator)
	agentpassword.RegisterExport(e.coordinator)
	status.RegisterExport(e.coordinator, e.clock, e.logger.Child("status"))
	resource.RegisterExport(e.coordinator, e.clock, e.logger.Child("resource"))
	cloudimagemetadata.RegisterExport(e.coordinator, e.logger.Child("cloudimagemetadata"), e.clock)
	model.RegisterExport(e.coordinator, e.logger.Child("model"))
	unitstate.RegisterExport(e.coordinator)

	// model agent must come after machine and unit
	modelagent.RegisterExport(e.coordinator, e.logger.Child("modelagent"))
}
