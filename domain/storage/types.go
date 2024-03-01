// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// Attrs defines storage attributes.
type Attrs map[string]string

// StoragePoolDetails defines the details of a storage pool to save.
// This type is also used when returning query results from state.
type StoragePoolDetails struct {
	Name     string
	Provider string
	Attrs    Attrs
}

// StoragePoolFilter defines attributes used to filter storage pools.
type StoragePoolFilter struct {
	// Names are pool's names to filter on.
	Names []string
	// Providers are pool's storage provider types to filter on.
	Providers []string
}
