// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// These structs represent the persistent storage pool entity schema in the database.
type storagePoolIdentifiers struct {
	// UUID is the unique identifier for the storage pool.
	UUID string `db:"uuid"`
	// Name is the name of the storage pool.
	Name string `db:"name"`
}

type storagePool struct {
	ID string `db:"uuid"`

	Name         string `db:"name"`
	ProviderType string `db:"type"`
}

type poolAttribute struct {
	// ID holds the cloud uuid.
	ID string `db:"storage_pool_uuid"`

	// Key is the key value.
	Key string `db:"key"`

	// Value is the value associated with key.
	Value string `db:"value"`
}

type storagePoolNames []string

type storageProviderTypes []string

type storagePools []storagePool

func (rows storagePools) toStoragePools(keyValues []poolAttribute) ([]storage.StoragePool, error) {
	if n := len(rows); n != len(keyValues) {
		// Should never happen.
		return nil, errors.New("row length mismatch")
	}

	var (
		result  []storage.StoragePool
		current *storage.StoragePool
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
		if current != nil && current.UUID != row.ID {
			// We have a new storage pool, so push the current one to the result.
			pushResult()
		}

		if current == nil {
			current = &storage.StoragePool{
				UUID:     row.ID,
				Name:     row.Name,
				Provider: row.ProviderType,
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
