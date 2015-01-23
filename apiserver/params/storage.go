// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// StorageInstance holds data for a storage instance.
type StorageInstance struct {
	StorageTag    string
	OwnerTag      string
	Location      string
	StorageName   string
	AvailableSize uint64
	TotalSize     uint64
	Tags          []string
}

// StorageInstancesResult holds a collection of storage instances.
type StorageInstancesResult struct {
	Results []StorageInstance
}
