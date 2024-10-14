// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
)

// MigrationMachineDiscrepancy describes a divergent machine between what Juju
// has and what the cloud has reported. If both the MachineName and the
// CloudInstanceId are both not empty then the discrepancy is on the Juju side
// where we are referencing a instance that doesn't exist in the cloud.
//
// If MachineName is empty then the discrepancy comes from the cloud where a
// instance exists that is not being tracked by Juju.
type MigrationMachineDiscrepancy struct {
	// MachineName is the name given to a machine in the Juju model
	MachineName machine.Name

	// CloudInstanceId is the unique id given to an instance from the cloud.
	CloudInstanceId instance.Id
}

// ModelMigrationInfo holds the information about a model in relation to the
// controller.
type ModelMigrationInfo struct {
	// IsControllerModel is true if the model is a controller model.
	IsControllerModel bool
	// ControllerUUID is the UUID of the controller.
	ControllerUUID controller.UUID
	// MigrationActive boolean to indicate if the model is currently in a
	// migration.
	MigrationActive bool
}
