// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	internalstorage "github.com/juju/juju/internal/storage"
)

// ImportStorageParams contains the parameters for importing storage into a model.
type ImportStorageParams struct {
	// Kind is the kind of the storage to import.
	Kind internalstorage.StorageKind

	// Pool is the name of the storage pool into which the storage is to
	// be imported.
	Pool string

	// ProviderId is the storage provider's unique ID for the storage,
	// e.g. the EBS volume ID.
	ProviderId string

	// StorageName is the name to assign to the imported storage.
	StorageName internalstorage.Name
}
