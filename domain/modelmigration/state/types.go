// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// ModelInfo represents the model's read only information from the model table
// in the model database.
type ModelInfo struct {
	// ControllerUUID is the controllers unique id.
	ControllerUUID string `db:"controller_uuid"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}
