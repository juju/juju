// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// StorageScope represents the storage scope
// as recorded in the storage scope lookup table.
type StorageScope int

const (
	// StorageScopeModel storage must be managed by a model level
	// storage provisioner.
	StorageScopeModel StorageScope = iota
	// StorageScopeHost storage must be managed from within the host.
	StorageScopeHost
)
