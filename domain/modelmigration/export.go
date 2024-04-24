// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	blockdevice "github.com/juju/juju/domain/blockdevice/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	modelconfig "github.com/juju/juju/domain/modelconfig/modelmigration"
	network "github.com/juju/juju/domain/network/modelmigration"
	storage "github.com/juju/juju/domain/storage/modelmigration"
	internalstorage "github.com/juju/juju/internal/storage"
)

// ExportOperations registers the export operations with the given coordinator.
// This is a convenience function that can be used by the main migration package
// to register all the export operations.
func ExportOperations(coordinator Coordinator, registry internalstorage.ProviderRegistry, logger Logger) {
	// Note: All the export operations are registered here.
	modelconfig.RegisterExport(coordinator)
	credential.RegisterExport(coordinator)
	network.RegisterExport(coordinator, logger)
	externalcontroller.RegisterExport(coordinator)
	// When machines are handled, they need to be done before block devices.
	blockdevice.RegisterExport(coordinator)
	storage.RegisterExport(coordinator, registry)
}
