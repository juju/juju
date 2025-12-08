package internal

import (
	domainstorage "github.com/juju/juju/domain/storage"
)

// CreateStoragePool represents a set of args used internally to the domain for
// creating a new storage pool.
type CreateStoragePool struct {
	Attrs        map[string]string
	Name         string
	ProviderType domainstorage.ProviderType
	UUID         domainstorage.StoragePoolUUID
}
