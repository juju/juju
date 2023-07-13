// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
)

// ExportOperations registers the export operations with the given coordinator.
// This is a convenience function that can be used by the main migration package
// to register all the export operations.
func ExportOperations(coordinator Coordinator, logger Logger) {
	// Note: All the export operations are registered here.
	externalcontroller.RegisterExport(coordinator)
}
