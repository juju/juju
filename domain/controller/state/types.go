// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// controllerModelUUID is used to fetch the controllerModelUUID from the database.
type controllerModelUUID struct {
	UUID string `db:"model_uuid"`
}

type controllerControllerAgentInfo struct {
	APIPort        int    `db:"api_port"`
	Cert           string `db:"cert"`
	PrivateKey     string `db:"private_key"`
	CAPrivateKey   string `db:"ca_private_key"`
	SystemIdentity string `db:"system_identity"`
}
