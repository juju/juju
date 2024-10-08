// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// modelInfo represents the model's read only information from the model table
// in the model database.
type modelInfo struct {
	// ControllerUUID is the controllers unique id.
	ControllerUUID string `db:"controller_uuid"`
}

// modelControllerInfo represents the model's information in relation to the
// controller.
type modelControllerInfo struct {
	// ControllerUUID is the UUID of the controller.
	ControllerUUID string `db:"controller_uuid"`
	// IsControllerModel is a boolean value to determine if the model is the
	// controller model.
	IsControllerModel bool `db:"is_controller_model"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}
