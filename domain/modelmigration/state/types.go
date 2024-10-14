// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/domain/life"

// modelInfo represents the model's read only information from the model table
// in the model database.
type modelInfo struct {
	// ControllerUUID is the controllers unique id.
	ControllerUUID string `db:"controller_uuid"`
}

// modelMigrationInfo represents the model's information in relation to the
// controller.
type modelMigrationInfo struct {
	// UUID is the unique id of the model.
	UUID string `db:"uuid"`
	// ControllerUUID is the UUID of the controller.
	ControllerUUID string `db:"controller_uuid"`
	// ControllerModelUUID is the UUID of the controller model.
	ControllerModelUUID string `db:"controller_model_uuid"`
	// MigrationActive is a boolean value to determine if the model is currently
	// in a migration.
	MigrationActive bool `db:"migration_active"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}

// modelLife represents the struct to be used for the life column within the
// sqlair statements in the model domain.
type modelLife struct {
	Life life.Life `db:"life_id"`
}

// modelUUID represents the struct to be used for the uuid column within the
// sqlair statements in the model domain.
type modelUUID struct {
	UUID string `db:"uuid"`
}

// userName is used to pass a user's name as an argument to SQL.
type userName struct {
	Name string `db:"name"`
}

// userUUID is used to pass a user's UUID as an argument to SQL.
type userUUID struct {
	UUID string `db:"uuid"`
}
