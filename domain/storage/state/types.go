// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// entityUUID represents the UUID of a storage entity in the model.
type entityUUID struct {
	UUID string `db:"uuid"`
}

// storageInstanceID represents the storage instance storage_id column for a
// row in the storage_instance table.
type storageInstanceID struct {
	ID string `db:"storage_id"`
}

// storageInstanceUUID represents the UUID of a storage instance in the model.
type storageInstanceUUID entityUUID

// unitUUID represents the UUID of a unit in the model.
type unitUUID entityUUID
