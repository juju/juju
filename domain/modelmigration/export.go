// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/juju/core/logger"
	access "github.com/juju/juju/domain/access/modelmigration"
	application "github.com/juju/juju/domain/application/modelmigration"
	blockdevice "github.com/juju/juju/domain/blockdevice/modelmigration"
	cloudimagemetadata "github.com/juju/juju/domain/cloudimagemetadata/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	keymanager "github.com/juju/juju/domain/keymanager/modelmigration"
	machine "github.com/juju/juju/domain/machine/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	network "github.com/juju/juju/domain/network/modelmigration"
	secret "github.com/juju/juju/domain/secret/modelmigration"
	storage "github.com/juju/juju/domain/storage/modelmigration"
	internalstorage "github.com/juju/juju/internal/storage"
)

// ExportOperations registers the export operations with the given coordinator.
// This is a convenience function that can be used by the main migration package
// to register all the export operations.
func ExportOperations(
	coordinator Coordinator,
	registry internalstorage.ProviderRegistry,
	logger logger.Logger,
) {
	modelconfig.RegisterExport(coordinator)
	access.RegisterExport(coordinator, logger.Child("access"))
	keymanager.RegisterExport(coordinator)
	credential.RegisterExport(coordinator, logger.Child("credential"))
	network.RegisterExport(coordinator, logger.Child("network"))
	externalcontroller.RegisterExport(coordinator)
	machine.RegisterExport(coordinator, logger.Child("machine"))
	blockdevice.RegisterExport(coordinator, logger.Child("blockdevice"))
	storage.RegisterExport(coordinator, registry, logger.Child("storage"))
	secret.RegisterExport(coordinator, logger.Child("secret"))
	application.RegisterExport(coordinator, logger.Child("application"))
	cloudimagemetadata.RegisterExport(coordinator, logger.Child("cloudimagemetadata"))
}
