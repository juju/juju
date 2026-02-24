// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// insertStoragePoolAttribute represents the sqlair type for inserting a new
// storage pool attribute record into the storage_pool_attribute table.
type insertStoragePoolAttribute struct {
	Key             string `db:"key"`
	StoragePoolUUID string `db:"storage_pool_uuid"`
	Value           string `db:"value"`
}

// insertStoragePool represents the sqlair type for inserting a new storage pool
// record into the storage_pool table.
type insertStoragePool struct {
	OriginID int    `db:"origin_id"`
	Name     string `db:"name"`
	Type     string `db:"type"`
	UUID     string `db:"uuid"`
}

// These structs represent the persistent storage pool entity schema in the database.

// storagePool represents a single storage pool record from the storage_pool
// table.
type storagePool struct {
	// OriginID represents the origin of the storage pool and helps provide
	// context for how the storage pool came to be.
	OriginID int `db:"origin_id"`

	// Name is the unique name of the storage pool within the model.
	Name string `db:"name"`

	// Type is the provider type of the entity responsible for provisioning
	// storage created with this pool in the model.
	Type string `db:"type"`

	// UUID is the unique id given to this storage pool in the model.
	UUID string `db:"uuid"`
}

// storagePoolAttribute represents a single storage pool attribute record from
// the storage_pool_attribute table.
type storagePoolAttribute struct {
	// UUID is the unique id of the storage pool in the model.
	UUID string `db:"storage_pool_uuid"`

	// Key is the unique key of a single storage pool attribute.
	Key string `db:"key"`

	// Value is the value associated with [storagePoolAttribute.Key].
	Value string `db:"value"`
}

// storagePoolName represents a storage pools name within the model.
type storagePoolName struct {
	Name string `db:"name"`
}

type storagePoolNameAndType struct {
	Name string `db:"name"`
	Type string `db:"type"`
}

type storagePoolNames []string

type storageProviderTypes []string

type storagePools []storagePool

func (rows storagePools) toStoragePools(keyValues []storagePoolAttribute) ([]domainstorage.StoragePool, error) {
	if n := len(rows); n != len(keyValues) {
		// Should never happen.
		return nil, errors.New("row length mismatch")
	}

	var (
		result  []domainstorage.StoragePool
		current *domainstorage.StoragePool
	)

	pushResult := func() {
		if current == nil {
			return
		}
		current.UUID = ""
		result = append(result, *current)
		current = nil
	}

	for i, row := range rows {
		if current != nil && current.UUID != row.UUID {
			// We have a new storage pool, so push the current one to the result.
			pushResult()
		}

		if current == nil {
			current = &domainstorage.StoragePool{
				UUID:     row.UUID,
				Name:     row.Name,
				Provider: row.Type,
			}
		}

		if keyValues[i].Key != "" {
			if current.Attrs == nil {
				current.Attrs = make(map[string]string)
			}
			current.Attrs[keyValues[i].Key] = keyValues[i].Value
		}
	}
	// Push the last storage pool if it exists.
	pushResult()
	return result, nil
}
