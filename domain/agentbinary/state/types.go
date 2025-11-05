// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/domain/agentbinary"

// agentStoreBinary represents an agent binary that exists within the object
// store.
type agentStoreBinary struct {
	ArchitectureID int    `db:"architecture_id"`
	SHA256         string `db:"sha_256"`
	Size           int64  `db:"size"`
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

type metadataRecord struct {
	// Version is the version of the agent binary.
	Version string `db:"version"`
	// Arch is the architecture of the agent binary.
	Arch string `db:"architecture_name"`
	// Size is the size of the agent binary in bytes.
	Size int64 `db:"size"`
	// SHA256 is the SHA256 hash of the agent binary.
	SHA256 string `db:"sha_256"`
}

// modelAgentStream represents the stream in use for the agent.
type modelAgentStream struct {
	StreamID int `db:"stream_id"`
}

type metadataRecords []metadataRecord

func (m metadataRecords) toMetadata() []agentbinary.Metadata {
	metadata := make([]agentbinary.Metadata, len(m))
	for i, record := range m {
		metadata[i] = agentbinary.Metadata{
			Version: record.Version,
			Arch:    record.Arch,
			Size:    record.Size,
			SHA256:  record.SHA256,
		}
	}
	return metadata
}
