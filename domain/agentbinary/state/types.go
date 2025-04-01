// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// architectureRecord represents an architecture row in the database.
type architectureRecord struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// objectStoreUUID is a database type for representing the uuid of an object
// store metadata row.
type objectStoreUUID struct {
	UUID string `db:"uuid"`
}

// agentBinaryRecord represents an agent binary entry in the database.
type agentBinaryRecord struct {
	Version         string `db:"version"`
	ArchitectureID  int    `db:"architecture_id"`
	ObjectStoreUUID string `db:"object_store_uuid"`
}
