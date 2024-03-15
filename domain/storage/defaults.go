// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	coremodel "github.com/juju/juju/core/model"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

// StorageDefaults holds the default sources of storage for an application.
type StorageDefaults struct {
	DefaultBlockSource      *string
	DefaultFilesystemSource *string
}

func storageKind(storageType charm.StorageType) storage.StorageKind {
	kind := storage.StorageKindUnknown
	switch storageType {
	case charm.StorageBlock:
		kind = storage.StorageKindBlock
	case charm.StorageFilesystem:
		kind = storage.StorageKindFilesystem
	}
	return kind
}

// StorageConstraintsWithDefaults returns a constraints
// derived from cons, with any defaults filled in.
func StorageConstraintsWithDefaults(
	charmStorage map[string]charm.Storage,
	modelType coremodel.ModelType,
	defaults StorageDefaults,
	allCons map[string]storage.Constraints,
) error {
	for name, storage := range charmStorage {
		cons, ok := allCons[name]
		if !ok {
			if storage.Shared {
				// TODO(axw) get the model's default shared storage pool, and create constraints here.
				return fmt.Errorf(
					"%w for shared charm storage %q",
					storageerrors.MissingSharedStorageConstraintError,
					name,
				)
			}
		}
		cons, err := storageConstraintsWithDefaults(storage, modelType, defaults, cons)
		if err != nil {
			return errors.Trace(err)
		}
		// Replace in case pool or size were updated.
		allCons[name] = cons
	}
	return nil
}

func storageConstraintsWithDefaults(
	charmStorage charm.Storage,
	modelType coremodel.ModelType,
	defaults StorageDefaults,
	cons storage.Constraints,
) (storage.Constraints, error) {
	withDefaults := cons

	// If no pool is specified, determine the pool from the env config and other constraints.
	if cons.Pool == "" {
		kind := storageKind(charmStorage.Type)
		poolName, err := defaultStoragePool(defaults, modelType, kind, cons)
		if err != nil {
			return withDefaults, errors.Annotatef(err, "finding default pool for %q storage", charmStorage.Name)
		}
		withDefaults.Pool = poolName
	}

	// If no size is specified, we default to the min size specified by the
	// charm, or 1GiB.
	if cons.Size == 0 {
		if charmStorage.MinimumSize > 0 {
			withDefaults.Size = charmStorage.MinimumSize
		} else {
			withDefaults.Size = 1024
		}
	}
	if cons.Count == 0 {
		withDefaults.Count = uint64(charmStorage.CountMin)
	}
	return withDefaults, nil
}

func storagePool(pool *string, fallbacks ...*string) string {
	if pool != nil {
		return *pool
	}
	for _, f := range fallbacks {
		if f == nil {
			continue
		}
		return *f
	}
	return ""
}

// defaultStoragePool returns the default storage pool for the model.
// The default pool is either user specified, or one that is registered by the provider itself.
func defaultStoragePool(
	defaults StorageDefaults,
	modelType coremodel.ModelType,
	kind storage.StorageKind,
	cons storage.Constraints,
) (string, error) {
	switch kind {
	case storage.StorageKindBlock:
		fallbackPool := string(provider.LoopProviderType)
		if modelType == coremodel.CAAS {
			fallbackPool = string(k8sconstants.StorageProviderType)
		}

		emptyConstraints := storage.Constraints{}
		if cons == emptyConstraints {
			// No constraints at all: use fallback.
			return fallbackPool, nil
		}
		// Either size or count specified, use env default.
		return storagePool(defaults.DefaultBlockSource, &fallbackPool), nil

	case storage.StorageKindFilesystem:
		fallbackPool := string(provider.RootfsProviderType)
		if modelType == coremodel.CAAS {
			fallbackPool = string(k8sconstants.StorageProviderType)
		}
		emptyConstraints := storage.Constraints{}
		if cons == emptyConstraints {
			return fallbackPool, nil
		}

		// If a filesystem source is specified in config,
		// use that; otherwise if a block source is specified,
		// use that and create a filesystem within.
		return storagePool(defaults.DefaultFilesystemSource, defaults.DefaultBlockSource, &fallbackPool), nil
	}
	return "", storageerrors.ErrNoDefaultStoragePool
}
