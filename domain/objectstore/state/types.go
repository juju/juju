// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	coreobjectstore "github.com/juju/juju/core/objectstore"
)

// dbMetadata represents the database serialisable metadata for an object.
type dbMetadata struct {
	// UUID is the uuid for the metadata.
	UUID string `db:"uuid"`
	// SHA256 is the 256 hash of the object.
	SHA256 string `db:"sha_256"`
	// SHA384 is the 512-384 hash of the object.
	SHA384 string `db:"sha_384"`
	// Path is the path to the object.
	Path string `db:"path"`
	// Size is the size of the object.
	Size int64 `db:"size"`
}

type sha256Ident struct {
	// SHA256 is the prefix 256 hash of the object.
	SHA256 string `db:"sha_256"`
}

type sha256IdentPrefix struct {
	// SHA256Prefix is the prefix 256 hash of the object.
	SHA256Prefix string `db:"sha_256_prefix"`
}

// dbMetadataPath represents the database serialisable metadata path for an
// object.
type dbMetadataPath struct {
	// UUID is the uuid for the metadata.
	UUID string `db:"metadata_uuid"`
	// Path is the path to the object.
	Path string `db:"path"`
}

// ToCoreObjectStoreMetadata transforms de-serialised data from the database to
// object metadata.
func decodeDbMetadata(m dbMetadata) coreobjectstore.Metadata {
	return coreobjectstore.Metadata{
		SHA256: m.SHA256,
		SHA384: m.SHA384,
		Path:   m.Path,
		Size:   m.Size,
	}
}

type dbGetPhaseInfo struct {
	// UUID is the uuid for the phase info.
	UUID string `db:"uuid"`
	// Phase is the phase of the object store.
	Phase coreobjectstore.Phase `db:"phase"`
}

type dbSetPhaseInfo struct {
	// UUID is the uuid for the phase info.
	UUID string `db:"uuid"`
	// PhaseTypeID is the phase of the object store.
	PhaseTypeID int `db:"phase_type_id"`
}
