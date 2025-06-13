// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// These structs represent the persistent storage pool entity schema in the database.

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

	var result []storage.StoragePool
	recordResult := func(row *storagePool, attrs poolAttributes) {
		result = append(result, storage.StoragePool{
			Name:     row.Name,
			Provider: row.ProviderType,
			Attrs:    storage.Attrs(attrs),
		})
	}

	var (
		current *storagePool
		attrs   poolAttributes
	)
	for i, row := range rows {
		if current != nil && row.ID != current.ID {
			recordResult(current, attrs)
			attrs = nil
		}
		if keyValues[i].Key != "" {
			if attrs == nil {
				attrs = make(poolAttributes)
			}
			attrs[keyValues[i].Key] = keyValues[i].Value
		}
		rowCopy := row
		current = &rowCopy
	}
	if current != nil {
		recordResult(current, attrs)
	}
	return result, nil
}
