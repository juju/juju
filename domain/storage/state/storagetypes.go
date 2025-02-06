// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent storage schema in the database.

type ModelDetails struct {
	ModelUUID      string `db:"uuid"`
	ControllerUUID string `db:"controller_uuid"`
}
