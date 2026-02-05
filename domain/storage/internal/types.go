// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// ImportStorageInstanceArgs represents data to import a storage instance
// and its owner.
type ImportStorageInstanceArgs struct {
	UUID             string
	Life             int
	PoolName         string
	RequestedSizeMiB uint64
	StorageName      string
	StorageKind      string
	StorageID        string
	UnitName         string
}
