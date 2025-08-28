// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// providerDefaultStoragePoolUUIDs returns the fixed storage pool uuid to use
// for a default storage pool.
//
// The following errors may be expected:
// - coreerrors.NotFound when no storage pool uuid exists for the pool name and
// provider type tuple.
func providerDefaultStoragePoolUUIDs(
	poolName, providerType string,
) (StoragePoolUUID, error) {
	// These fixed values are offered via a static function interface because
	// maps can not be const values in Go. Having this as a variable invites
	// fiddling.

	// lookupTuple is the comparable type for which fixed storage pool uuids are
	// stored for.
	type lookupTuple struct {
		// N is the name of the storage pool.
		N string

		// T is the provider type of the storage pool.
		T string
	}

	defaultPoolUUIDs := map[lookupTuple]StoragePoolUUID{
		lookupTuple{N: "loop", T: "loop"}: "",
	}

	lookup := lookupTuple{
		N: poolName,
		T: providerType,
	}

	uuid, exists := defaultPoolUUIDs[lookup]
	if !exists {
		return "", errors.Errorf(
			"no default storage pool uuid exists for pool %q of type %q",
			poolName, providerType,
		).Add(coreerrors.NotFound)
	}

	return uuid, nil
}
