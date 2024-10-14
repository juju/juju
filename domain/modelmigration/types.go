// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
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

// CreateMigrationArgs holds the arguments for creating a migration record in
// the state of the model that is being migrated.
type CreateMigrationArgs struct {
	// ModelUUID is the UUID of the model that is being migrated.
	ModelUUID model.UUID

	// ControllerUUID holds tag for the target controller.
	ControllerUUID controller.UUID

	// ControllerAlias holds an optional alias for the target controller.
	ControllerAlias string

	// Addrs holds the addresses and ports of the target controller's
	// API servers.
	Addrs []string

	// CACert holds the CA certificate that will be used to validate
	// the target API server's certificate, in PEM format.
	CACert string

	// AuthTag holds the user tag to authenticate with to the target
	// controller.
	UserUUID user.UUID

	// Password holds the password to use with AuthTag.
	Password string

	// Macaroons holds macaroons to use with UserUUID. At least one of
	// Password or Macaroons must be set.
	Macaroons []byte
}
