// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/domain/application/internal"
	domainstorage "github.com/juju/juju/domain/storage"
)

// CalculateStorageInstanceSizeForAttachment returns the size in MiB to use
// when validating a storage instance for attachment. Filesystem or volume
// provisioned sizes are used when set; otherwise the requested size is used.
// If the kind is unknown, the requested size is used.
func CalculateStorageInstanceSizeForAttachment(
	info internal.StorageInstanceInfo,
) uint64 {
	fallback := max(info.RequestedSizeMIB, 0)

	switch info.Kind {
	case domainstorage.StorageKindFilesystem:
		if info.Filesystem != nil && info.Filesystem.SizeMib != 0 {
			return info.Filesystem.SizeMib
		}
	case domainstorage.StorageKindBlock:
		if info.Volume != nil && info.Volume.SizeMiB != 0 {
			return info.Volume.SizeMiB
		}
	}
	return fallback
}
