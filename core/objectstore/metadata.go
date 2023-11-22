// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

// Metadata represents the metadata for an object.
type Metadata struct {
	// UUID is the unique identifier for the metadata.
	UUID string
	// Key is the key for the metadata, it's a temporary field until we
	// have uuids everywhere.
	Key string
	// Hash is the hash of the object.
	Hash string
	// Path is the path to the object.
	Path string
	// Size is the size of the object.
	Size int64
}
