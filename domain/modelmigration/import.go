// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/juju/core/modelmigration"
	credential "github.com/juju/juju/domain/credential/modelmigration"
	externalcontroller "github.com/juju/juju/domain/externalcontroller/modelmigration"
	lease "github.com/juju/juju/domain/lease/modelmigration"
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
func ImportOperations(coordinator Coordinator, logger Logger) {
	// Note: All the import operations are registered here.
	lease.RegisterImport(coordinator, logger)
	externalcontroller.RegisterImport(coordinator)
	credential.RegisterImport(coordinator)
}
