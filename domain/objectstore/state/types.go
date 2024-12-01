// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import coreobjectstore "github.com/juju/juju/core/objectstore"

// dbMetadata represents the database serialisable metadata for an object.
type dbMetadata struct {
	// UUID is the uuid for the metadata.
	UUID string `db:"uuid"`
	// Hash256 is the 256 hash of the object.
	Hash256 string `db:"hash_256"`
	// Hash512_384 is the 512_384 hash of the object.
	Hash512_384 string `db:"hash_512_384"`
	// Path is the path to the object.
	Path string `db:"path"`
	// Size is the size of the object.
	Size int64 `db:"size"`
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
func (m dbMetadata) ToCoreObjectStoreMetadata() coreobjectstore.Metadata {
	return coreobjectstore.Metadata{
		Hash256:     m.Hash256,
		Hash512_384: m.Hash512_384,
		Path:        m.Path,
		Size:        m.Size,
	}
}
