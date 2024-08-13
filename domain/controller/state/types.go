// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// controllerModelUUID is used to fetch the controllerModelUUID from the database.
type controllerModelUUID struct {
	UUID string `db:"model_uuid"`
}
