// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// agentStoreBinary represents an agent binary that exists within the object
// store.
type agentStoreBinary struct {
	ArchitectureID int    `db:"architecture_id"`
	SHA256         string `db:"sha_256"`
	Size           uint64 `db:"size"`
	StreamID       int    `db:"stream_id"`
	Version        string `db:"version"`
}

// agentStream represents the stream in use for the agent.
type agentStream struct {
	StreamID int `db:"stream_id"`
}

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

// objectStoreSHA256Sum is a database type for representing the sha 256 sum
// of an object store row.
type objectStoreSHA256Sum struct {
	Sum string `db:"sha_256"`
}

// agentBinaryRecord represents an agent binary entry in the database.
type agentBinaryRecord struct {
	Version         string `db:"version"`
	ArchitectureID  int    `db:"architecture_id"`
	ObjectStoreUUID string `db:"object_store_uuid"`
}

// modelAgentStream represents the stream in use for the agent.
type modelAgentStream struct {
	StreamID int `db:"stream_id"`
}
