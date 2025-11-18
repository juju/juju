// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// modelStatusContext represents a single row from the v_model_state view.
// These information are used to determine a model's status.
type modelStatusContext struct {
	Destroying              bool   `db:"destroying"`
	CredentialInvalid       bool   `db:"cloud_credential_invalid"`
	CredentialInvalidReason string `db:"cloud_credential_invalid_reason"`
	Migrating               bool   `db:"migrating"`
}

type modelUUID struct {
	UUID string `db:"uuid"`
}

// controllerID is the database representation of a controller node id.
type controllerID struct {
	ControllerID string `db:"controller_id"`
	DqliteNodeID uint64 `db:"dqlite_node_id"`
}
