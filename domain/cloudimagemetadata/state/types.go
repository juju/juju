// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "time"

// inputMetadata represents metadata information for a cloud image.
// Its only purpose is to inject data in sql statement or extract query results.
type inputMetadata struct {
	UUID            string    `db:"uuid"`
	CreatedAt       time.Time `db:"created_at"`
	Source          string    `db:"source"`
	Stream          string    `db:"stream"`
	Region          string    `db:"region"`
	Version         string    `db:"version"`
	VirtType        string    `db:"virt_type"`
	RootStorageType string    `db:"root_storage_type"`
	RootStorageSize *uint64   `db:"root_storage_size"`
	Priority        int       `db:"priority"`
	ArchitectureID  int       `db:"architecture_id"`
	ImageID         string    `db:"image_id"`
}

// outputMetadata represents metadata information related to a cloud image.
// Its only purpose is to inject data in sql statement or extract query results.
type outputMetadata struct {
	CreatedAt        time.Time `db:"created_at"`
	Source           string    `db:"source"`
	Stream           string    `db:"stream"`
	Region           string    `db:"region"`
	Version          string    `db:"version"`
	VirtType         string    `db:"virt_type"`
	RootStorageType  string    `db:"root_storage_type"`
	RootStorageSize  *uint64   `db:"root_storage_size"`
	Priority         int       `db:"priority"`
	ArchitectureName string    `db:"architecture_name"`
	ImageID          string    `db:"image_id"`
}

// metadataImageID represents a structure holding an image ID for metadata processing in the database.
// Its only purpose is to inject data in sql statement or extract query results.
type metadataImageID struct {
	ID string `db:"image_id"`
}

// metadataUUID represents a structure that encapsulates a universally unique identifier (UUID) for metadata purposes.
// Its only purpose is to inject data in sql statement or extract query results.
type metadataUUID struct {
	UUID string `db:"uuid"`
}

// ttl represents a Time-To-Live (TTL) structure used to specify expiration time.
// Its only purpose is to inject data in sql statement or extract query results.
type ttl struct {
	ExpiresAt time.Time `db:"expires_at"`
}

// metadataFilter contains all metadata attributes that allows to find a particular
// cloud image metadata. Since size and source are not discriminating attributes
// for cloud image metadata, they are not included in search criteria.
// Its only purpose is to inject data in sql statement or extract query results.
type metadataFilter struct {
	Region          string   `db:"region"`
	Versions        versions `db:"versions"`
	Arches          arches   `db:"architecture_names"`
	Stream          string   `db:"stream"`
	VirtType        string   `db:"virt_type"`
	RootStorageType string   `db:"root_storage_type"`
}

// versions represents a list of version strings in the metadataFilter struct.
type versions []string

// arches represents a list of architecture names in the metadataFilter struct.
type arches []string
