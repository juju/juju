// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// entityUUID represents the UUID of a storage entity in the model.
type entityUUID struct {
	UUID string `db:"uuid"`
}

// nameAndUUID is an agnostic container for a `name` and `uuid`
// column combination.
type nameAndUUID struct {
	Name string `db:"name"`
	UUID string `db:"uuid"`
}

// name is an agnostic container for a `name` value.
type name struct {
	Name string `db:"name"`
}

// idAndKind represents an agnostic container for `id` and `kind`
// column combination.
type idAndKind struct {
	ID   int    `db:"id"`
	Kind string `db:"kind"`
}

// storageInstanceID represents the storage instance storage_id column for a
// row in the storage_instance table.
type storageInstanceID struct {
	ID string `db:"storage_id"`
}

type storageInstanceUUIDAndID struct {
	UUID string `db:"uuid"`
	ID   string `db:"storage_id"`
}

type storageInstanceIDs []string

// dbModelStoragePool represents a single row from the model_storage_pool table.
type dbModelStoragePool struct {
	StoragePoolUUID string `db:"storage_pool_uuid"`
	StorageKindID   int    `db:"storage_kind_id"`
}

// dbAggregateCount is a type to store the result for counting the number of
// rows returned by a select query.
type dbAggregateCount struct {
	Count int `db:"count"`
}

// storageInstanceUUID represents the UUID of a storage instance in the model.
type storageInstanceUUID entityUUID

// unitUUID represents the UUID of a unit in the model.
type unitUUID entityUUID

// importStorageInstance represents a storage_instance.
type importStorageInstance struct {
	UUID            string `db:"uuid"`
	CharmName       string `db:"charm_name"`
	StorageName     string `db:"storage_name"`
	StorageID       string `db:"storage_id"`
	StorageKindID   int    `db:"storage_kind_id"`
	LifeID          int    `db:"life_id"`
	StoragePoolUUID string `db:"storage_pool_uuid"`
	RequestedSize   uint64 `db:"requested_size_mib"`
}

// importStorageUnitOwner represents a storage_unit_owner.
type importStorageUnitOwner struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	UnitUUID            string `db:"unit_uuid"`
}

type importStorageFilesystem struct {
	UUID       string `db:"uuid"`
	ID         string `db:"filesystem_id"`
	LifeID     int    `db:"life_id"`
	ScopeID    int    `db:"provision_scope_id"`
	ProviderID string `db:"provider_id"`
	SizeInMiB  uint64 `db:"size_mib"`
}

type importStorageInstanceFilesystem struct {
	StorageInstanceUUID string `db:"storage_instance_uuid"`
	FilesystemUUID      string `db:"storage_filesystem_uuid"`
}
