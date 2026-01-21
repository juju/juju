// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import domainstorage "github.com/juju/juju/domain/storage"

// StoragePoolToCreate describes a single storage pool that MUST be created as
// part of bootstrapping. The model where the storage pool is controlled by the
// user of this type.
type StoragePoolToCreate struct {
	// Attributes is the set of attributes to be applied with the storage pool.
	Attributes map[string]any

	// Name is the unique name in the model for the storage pool.
	Name string

	// ProviderType is the type of provider used by this storage pool for
	// provisioning storage in the model.
	ProviderType domainstorage.ProviderType
}
