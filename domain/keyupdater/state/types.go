// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// authorizedKey represents a single authorised key for a machine.
type authorizedKey struct {
	PublicKey string `db:"public_key"`
}

// keyValue represents a single row from the controllers config.
type keyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// machineName represents a single machine name
type machineName struct {
	Name string `db:"name"`
}

// modelUUIDValue represents a model id for associating public keys with.
type modelUUIDValue struct {
	UUID string `db:"model_uuid"`
}
