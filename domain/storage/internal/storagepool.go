// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainstorage "github.com/juju/juju/domain/storage"
)

// CreateStoragePool represents a set of args used internally to the domain for
// creating a new storage pool.
type CreateStoragePool struct {
	Attrs        map[string]string
	Name         string
	Origin       domainstorage.StoragePoolOrigin
	ProviderType domainstorage.ProviderType
	UUID         domainstorage.StoragePoolUUID
}
