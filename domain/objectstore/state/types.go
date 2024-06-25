// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import coreobjectstore "github.com/juju/juju/core/objectstore"

// dbMetadata represents the database serialisable metadata for an object.
type dbMetadata struct {
	// UUID is the uuid for the metadata.
	UUID string `db:"uuid"`
	// Hash is the hash of the object.
	Hash string `db:"hash"`
	// HashTypeID is the id of the type of hash used to generate the hash. It
	// can be looked up in object_store_metadata_hash_type.
	HashTypeID uint `db:"hash_type_id"`
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
		Hash: m.Hash,
		Path: m.Path,
		Size: m.Size,
	}
}
